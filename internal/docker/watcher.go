// File: internal/docker/watcher.go
package docker

import (
	"context"
	"io"
	"sync"
	"time"

	"github.com/ParsaSafavi05/deploycrane/internal/logging"
	model "github.com/ParsaSafavi05/deploycrane/internal/models"
	"github.com/ParsaSafavi05/deploycrane/internal/store"

	"github.com/moby/moby/api/types/events"
	"github.com/moby/moby/client"
)

type ContainerWatcher struct {
	cli   client.APIClient
	store store.Store

	mu         sync.RWMutex
	apps       map[string]string
	lastStatus map[string]model.Status
}

func NewContainerWatcher(cli client.APIClient, s store.Store) *ContainerWatcher {
	return &ContainerWatcher{
		cli:        cli,
		store:      s,
		apps:       make(map[string]string),
		lastStatus: make(map[string]model.Status),
	}
}

// RestoreMappings rebuilds container->app mappings from the DB and reconciles
// them against the current Docker daemon state.
func (w *ContainerWatcher) RestoreMappings(ctx context.Context) error {
	apps, err := w.store.List(ctx)
	if err != nil {
		return err
	}

	freshApps := make(map[string]string, len(apps))
	freshLast := make(map[string]model.Status, len(apps))
	restored := 0

	for _, app := range apps {
		if app.ContainerID == "" {
			continue
		}

		inspect, err := w.cli.ContainerInspect(ctx, app.ContainerID, client.ContainerInspectOptions{})
		if err != nil {
			logging.Warn("stale container detected", "app_id", app.ID, "container_id", app.ContainerID, err)

			if updateErr := w.store.Update(ctx, app.ID, func(a *model.App) {
				a.Status = model.StatusStopped
				a.ContainerID = ""
			}); updateErr != nil {
				logging.Error("failed clearing stale container for app", "app_id", app.ID, "container_id", app.ContainerID, "error", updateErr)
			}

			continue
		}

		status := model.StatusStopped
		if inspect.Container.State != nil && inspect.Container.State.Running {
			status = model.StatusRunning
		}

		freshApps[app.ContainerID] = app.ID
		freshLast[app.ID] = status
		restored++

		if updateErr := w.store.Update(ctx, app.ID, func(a *model.App) {
			a.Status = status
		}); updateErr != nil {
			logging.Error("failed syncing status for app", "app_id", app.ID, "error", updateErr)
		}

		logging.Debug("restored mapping", "container_id", app.ContainerID, "app_id", app.ID, "status", status)
	}

	w.mu.Lock()
	w.apps = freshApps
	w.lastStatus = freshLast
	w.mu.Unlock()

	logging.Info("watcher mappings restored", "count", restored)
	return nil
}

// Watch starts a single Docker event stream. If the stream ends or errors,
// the caller should reopen it.
func (w *ContainerWatcher) Watch(ctx context.Context) error {
	options := client.EventsListOptions{
		Filters: make(client.Filters),
	}

	options.Filters.Add("type", "container")
	options.Filters.Add("event", string(events.ActionStart))
	options.Filters.Add("event", string(events.ActionRestart))
	options.Filters.Add("event", string(events.ActionStop))
	options.Filters.Add("event", string(events.ActionDie))
	options.Filters.Add("event", string(events.ActionKill))
	options.Filters.Add("event", string(events.ActionDestroy))
	options.Filters.Add("event", string(events.ActionRemove))

	result := w.cli.Events(ctx, options)
	msgs := result.Messages
	errs := result.Err

	logging.Info("watcher listening for Docker container events")

	for msgs != nil || errs != nil {
		select {
		case msg, ok := <-msgs:
			if !ok {
				msgs = nil
				continue
			}
			w.handleEvent(ctx, msg)

		case err, ok := <-errs:
			if !ok {
				errs = nil
				continue
			}
			if err != nil {
				return err
			}

		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return io.EOF
}

// WatchLoop keeps the watcher alive and reconnects after stream failures.
func (w *ContainerWatcher) WatchLoop(ctx context.Context) {
	for {
		if err := w.Watch(ctx); err != nil && ctx.Err() == nil {
			logging.Warn("watcher event stream ended, reconnecting", "error", err)
		}

		if ctx.Err() != nil {
			return
		}

		select {
		case <-time.After(5 * time.Second):
		case <-ctx.Done():
			return
		}
	}
}

func (w *ContainerWatcher) handleEvent(ctx context.Context, msg events.Message) {
	containerID := msg.Actor.ID
	if containerID == "" {
		return
	}

	appID := w.getAppID(containerID)
	if appID == "" {
		return
	}

	status, clearMapping, ok := statusForAction(msg.Action)
	if !ok {
		return
	}

	w.mu.RLock()
	last := w.lastStatus[appID]
	w.mu.RUnlock()

	// Avoid redundant DB writes for repeated "running" or "stopped" events.
	if last == status && !clearMapping {
		return
	}

	if err := w.store.Update(ctx, appID, func(a *model.App) {
		a.Status = status
		if clearMapping {
			a.ContainerID = ""
		}
	}); err != nil {
		logging.Error("failed updating app status from event", "app_id", appID, "action", string(msg.Action), "error", err)
		return
	}

	w.mu.Lock()
	w.lastStatus[appID] = status
	if clearMapping {
		delete(w.apps, containerID)
	}
	w.mu.Unlock()

	logging.Info("app status changed", "app_id", appID, "status", status, "action", string(msg.Action))
}

// RegisterApp should be called right after a container is created for an app.
func (w *ContainerWatcher) RegisterApp(appID, containerID string) {
	if appID == "" || containerID == "" {
		return
	}

	w.mu.Lock()
	w.apps[containerID] = appID
	w.lastStatus[appID] = model.StatusRunning
	w.mu.Unlock()

	logging.Info("app registered to watcher", "app_id", appID, "container_id", containerID)
}

func (w *ContainerWatcher) getAppID(containerID string) string {
	w.mu.RLock()
	defer w.mu.RUnlock()

	return w.apps[containerID]
}

func statusForAction(action events.Action) (model.Status, bool, bool) {
	switch action {
	case events.ActionStart, events.ActionRestart:
		return model.StatusRunning, false, true

	case events.ActionStop, events.ActionDie, events.ActionKill:
		return model.StatusStopped, false, true

	case events.ActionDestroy, events.ActionRemove:
		return model.StatusStopped, true, true

	default:
		return "", false, false
	}
}