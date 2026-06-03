package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/ParsaSafavi05/deploycrane/internal/docker"
	"github.com/ParsaSafavi05/deploycrane/internal/git"
	"github.com/ParsaSafavi05/deploycrane/internal/logging"
	model "github.com/ParsaSafavi05/deploycrane/internal/models"
	"github.com/ParsaSafavi05/deploycrane/internal/sse"
)

// DeployApp runs the full deployment pipeline and streams progress
func (s *AppService) DeployApp(w io.Writer, ctx context.Context, app model.App) (model.App, error) {
	id := app.ID

	switch app.Status {
	case model.StatusCreated, model.StatusFailed,
		model.StatusCloned, model.StatusBuilt, model.StatusStopped:
	case model.StatusRunning:
		return app, fmt.Errorf("app already deployed")
	default:
		return app, fmt.Errorf("app is in a transitional state")
	}

	logging.Info("deploy started", "app_id", app.ID, "app_name", app.Name, "from_status", string(app.Status))
	// Step 1: Clone
	if app.Status == model.StatusCreated || app.Status == model.StatusFailed {
		updatedApp, err := s.CloneApp(ctx, app, w)
		if err != nil {
			sse.WriteEvent(w, "error", fmt.Sprintf("clone failed: %s", err.Error()))
			return app, err
		}
		app = updatedApp
		sse.WriteEvent(w, "", "Repository cloned successfully")
	}
	app, _ = s.store.Get(ctx, id)

	// Step 2: Build
	if app.Status != model.StatusBuilt && app.Status != model.StatusStopped {
		updatedApp, err := s.BuildApp(ctx, app, w)
		if err != nil {
			sse.WriteEvent(w, "error", fmt.Sprintf("build failed: %s", err.Error()))
			return app, err
		}
		app = updatedApp
	}

	// Step 3: Start
	if app.Status != model.StatusRunning {
		containerPort := app.ContainerPort
		if containerPort <= 0 {
			containerPort = s.cfg.ContainerPort
		}
		updatedApp, err := s.StartAppWithProgress(ctx, app, containerPort, app.HostPort, w)
		if err != nil {
			sse.WriteEvent(w, "error", fmt.Sprintf("start failed: %s", err.Error()))
			return app, err
		}
		app = updatedApp
	}

	sse.WriteEvent(w, "complete", "deploy finished — app is running")
	app, _ = s.store.Get(ctx, id)
	return app, nil
}

// CloneApp clones the repository for the given app and returns the updated app
func (s *AppService) CloneApp(ctx context.Context, app model.App, w io.Writer) (model.App, error) {
	if app.Status == model.StatusCloned {
		return app, fmt.Errorf("app already cloned")
	}
	if app.Status == model.StatusCloning {
		return app, fmt.Errorf("app already cloning")
	}
	if app.Status != model.StatusCreated && app.Status != model.StatusFailed {
		return app, fmt.Errorf("app is not in a cloneable state (current: %s)", app.Status)
	}

	if err := s.store.Update(ctx, app.ID, func(a *model.App) {
		logging.Info("clone started", "app_id", app.ID, "repo_url", app.RepoURL)
		a.Status = model.StatusCloning
	}); err != nil {
		logging.Error("clone failed", "app_id", app.ID, "repo_url", app.RepoURL, "error", err)
		return app, fmt.Errorf("failed to set status to cloning: %w", err)
	}

	if w != nil {
		sse.WriteEvent(w, "", fmt.Sprintf("cloning repository from %s...", app.RepoURL))
	}

	clonePath := fmt.Sprintf("%s/app-%s", s.cfg.CloneBasePath, app.ID)
	if err := git.Clone(ctx, app.RepoURL, clonePath); err != nil {
		_ = s.store.Update(ctx, app.ID, func(a *model.App) {
			a.Status = model.StatusFailed
		})
		return app, fmt.Errorf("clone failed: %w", err)
	}

	if w != nil {
		sse.WriteEvent(w, "", "clone complete")
	}

	if err := s.store.Update(ctx, app.ID, func(a *model.App) {
		a.Status = model.StatusCloned
		a.ClonePath = clonePath
		logging.Info("clone completed", "app_id", app.ID, "clone_path", clonePath)
	}); err != nil {
		return app, fmt.Errorf("clone succeeded but failed to update app: %w", err)
	}

	updated, err := s.store.Get(ctx, app.ID)
	if err != nil {
		return app, fmt.Errorf("clone succeeded but failed to fetch updated app: %w", err)
	}
	return updated, nil
}

// BuildApp builds the Docker image for the given app
func (s *AppService) BuildApp(ctx context.Context, app model.App, w io.Writer) (model.App, error) {
	if app.Status == model.StatusBuilt {
		return app, fmt.Errorf("app already built")
	}
	if app.Status == model.StatusBuilding {
		return app, fmt.Errorf("app already building")
	}
	if app.Status != model.StatusCloned && app.Status != model.StatusFailed {
		return app, fmt.Errorf("app is not ready for build (current: %s)", app.Status)
	}

	if err := s.store.Update(ctx, app.ID, func(a *model.App) {
		logging.Info("build started", "app_id", app.ID, "app_name", app.Name)
		a.Status = model.StatusBuilding
	}); err != nil {
		return app, fmt.Errorf("failed to set status to building: %w", err)
	}

	imageName := s.cfg.ImagePrefix + "-" + app.Name + ":latest"

	body, err := docker.ImageBuild(ctx, s.dockerClient, app.ClonePath, imageName)
	if err != nil {
		_ = s.store.Update(ctx, app.ID, func(a *model.App) {
			a.Status = model.StatusFailed
		})
		logging.Error("build failed", "app_id", app.ID, "error", err)
		return app, fmt.Errorf("build failed: %w", err)
	}
	defer body.Close()

	if w != nil {
		done := make(chan struct{})
		defer close(done)

		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		go func() {
			for {
				select {
				case <-ticker.C:
					sse.WriteEvent(w, "ping", "build in progress...")
				case <-done:
					return
				}
			}
		}()
		logging.Error("build log streaming failed", "app_id", app.ID, "error", err)
		if err := docker.StreamBuildLogs(w, body); err != nil {
			_ = s.store.Update(ctx, app.ID, func(a *model.App) {
				a.Status = model.StatusFailed
			})
			return app, fmt.Errorf("build failed: %w", err)
		}
	}

	if err := s.store.Update(ctx, app.ID, func(a *model.App) {
		logging.Info("build completed", "app_id", app.ID, "image", imageName)
		a.Status = model.StatusBuilt
	}); err != nil {
		return app, fmt.Errorf("build succeeded but failed to update status: %w", err)
	}

	updated, err := s.store.Get(ctx, app.ID)
	if err != nil {
		return app, fmt.Errorf("build succeeded but failed to fetch updated app: %w", err)
	}
	return updated, nil
}

// StartApp starts the app with no SSE progress reporting
func (s *AppService) StartApp(
	ctx context.Context,
	app model.App,
	containerPort int,
	userHostPort int,
) (model.App, error) {
	return s.startAppInternal(ctx, app, containerPort, userHostPort, nopReporter{})
}

// StartAppWithProgress starts the app and streams progress via SSE
func (s *AppService) StartAppWithProgress(
	ctx context.Context,
	app model.App,
	containerPort int,
	userHostPort int,
	w io.Writer,
) (model.App, error) {
	flusher, ok := w.(http.Flusher)
	if !ok || flusher == nil {
		return s.StartApp(ctx, app, containerPort, userHostPort)
	}
	return s.startAppInternal(ctx, app, containerPort, userHostPort, sseReporter{w: w})
}

func (s *AppService) startAppInternal(
	ctx context.Context,
	app model.App,
	containerPort int,
	userHostPort int,
	reporter ProgressReporter,
) (model.App, error) {
	emit := func(msg string) {
		if reporter != nil {
			reporter.Report(msg)
		}
	}

	emit("validating ports...")

	if containerPort <= 0 || containerPort > 65535 {
		return app, fmt.Errorf("invalid container port %d", containerPort)
	}

	emit("allocating host port...")

	hostPort, err := s.reserveHostPort(userHostPort)
	if err != nil {
		return app, err
	}

	if userHostPort > 0 && userHostPort <= 65535 && hostPort != userHostPort {
		emit(fmt.Sprintf("requested host port %d was unavailable; using %d...", userHostPort, hostPort))
	}

	emit(fmt.Sprintf("starting container (port %d → %d)...", containerPort, hostPort))

	if err := s.store.Update(ctx, app.ID, func(a *model.App) {
		a.Status = model.StatusStarting
	}); err != nil {
		return app, fmt.Errorf("failed to start app: %w", err)
	}

	imageName := s.cfg.ImagePrefix + "-" + app.Name + ":latest"
	portMappings := map[int]int{containerPort: hostPort}

	containerID, err := docker.StartContainer(ctx, s.dockerClient, imageName, portMappings)
	if err != nil {
		if updateErr := s.store.Update(ctx, app.ID, func(a *model.App) {
			a.Status = model.StatusFailed
		}); updateErr != nil {
			logging.Error("failed to mark app as failed after container start error", "app_id", app.ID, "error", updateErr)
		}
		return app, fmt.Errorf("failed to start container: %w", err)
	}

	emit("container created, registering watcher...")
	s.watcher.RegisterApp(app.ID, containerID)
	logging.Info("container started", "app_id", app.ID, "container_id", containerID, "host_port", hostPort)
	emit("updating database...")

	if err := s.store.Update(ctx, app.ID, func(a *model.App) {
		a.Status = model.StatusRunning
		a.ContainerID = containerID
		a.HostPort = hostPort
	}); err != nil {
		logging.Error("CRITICAL: container started but DB update failed", "app_id", app.ID, "error", err)
		return app, fmt.Errorf("container started but failed to update status: %w", err)
	}

	updatedApp, err := s.store.Get(ctx, app.ID)
	if err != nil {
		return app, fmt.Errorf("failed to fetch updated app: %w", err)
	}
	return updatedApp, nil
}

// StopApp stops the container and updates the db
func (s *AppService) StopApp(ctx context.Context, app model.App) (model.App, error) {
	if app.Status != model.StatusRunning {
		return app, fmt.Errorf("app is not running")
	}
	if app.ContainerID == "" {
		return app, fmt.Errorf("app has no active container")
	}

	_, err := docker.StopContainer(ctx, s.dockerClient, app.ContainerID)
	if err != nil {
		logging.Error("failed to stop container", "app_id", app.ID, "container_id", app.ContainerID, "error", err)
		return app, fmt.Errorf("failed to stop container: %s", err)
	}

	if err = s.store.Update(ctx, app.ID, func(a *model.App) {
		logging.Info("app stopped", "app_id", app.ID, "container_id", app.ContainerID)
		a.Status = model.StatusStopped
	}); err != nil {
		return app, fmt.Errorf("failed to update app status: %s", err)
	}

	updatedApp, err := s.store.Get(ctx, app.ID)
	if err != nil {
		return app, fmt.Errorf("app stopped but failed to refetch the app: %s", err)
	}
	return updatedApp, nil
}

// reserveHostPort handles port allocation logic
func (s *AppService) reserveHostPort(userHostPort int) (int, error) {
	userProvided := userHostPort > 0 && userHostPort <= 65535

	if userProvided {
		if s.pm.IsAvailable(userHostPort) {
			if err := s.pm.AllocateSpecific(userHostPort); err != nil {
				return 0, fmt.Errorf("failed to reserve port %d: %w", userHostPort, err)
			}
			return userHostPort, nil
		}
		allocated, err := s.pm.Allocate()
		if err != nil {
			return 0, fmt.Errorf("requested host port %d is unavailable and no free ports", userHostPort)
		}
		logging.Warn("requested port unavailable, allocated fallback", "requested", userHostPort, "allocated", allocated)
		return allocated, nil
	}

	allocated, err := s.pm.Allocate()
	if err != nil {
		return 0, fmt.Errorf("no free ports")
	}
	return allocated, nil
}

// ProgressReporter interface and implementations
type ProgressReporter interface {
	Report(string)
}

type nopReporter struct{}

func (nopReporter) Report(string) {}

type sseReporter struct {
	w io.Writer
}

func (r sseReporter) Report(msg string) {
	sse.WriteEvent(r.w, "", msg)
}
