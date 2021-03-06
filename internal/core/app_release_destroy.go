package core

import (
	"context"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/hashicorp/go-hclog"

	"github.com/hashicorp/waypoint-plugin-sdk/component"
	"github.com/hashicorp/waypoint-plugin-sdk/terminal"
	"github.com/hashicorp/waypoint/internal/config"
	pb "github.com/hashicorp/waypoint/internal/server/gen"
)

// CanDestroyRelease returns true if this app supports destroying releases.
func (a *App) CanDestroyRelease() bool {
	d, ok := a.Releaser.(component.Destroyer)
	return ok && d.DestroyFunc() != nil
}

// DestroyRelease destroyes a specific release.
func (a *App) DestroyRelease(ctx context.Context, d *pb.Release) error {
	// If the release is destroyed already then do nothing.
	if d.State == pb.Operation_DESTROYED {
		a.logger.Info("release already destroyed, doing nothing", "id", d.Id)
		return nil
	}

	_, _, err := a.doOperation(ctx, a.logger.Named("release"), &releaseDestroyOperation{
		Release: d,
	})
	return err
}

// destroyAllReleases will destroy all non-destroyed releases.
func (a *App) destroyAllReleases(ctx context.Context) error {
	resp, err := a.client.ListReleases(ctx, &pb.ListReleasesRequest{
		Application:   a.ref,
		Workspace:     a.workspace,
		PhysicalState: pb.Operation_CREATED,
	})
	if err != nil {
		return nil
	}

	rels := resp.Releases
	if len(rels) == 0 {
		return nil
	}

	a.UI.Output("Destroying releases...", terminal.WithHeaderStyle())
	for _, rel := range rels {
		err := a.DestroyRelease(ctx, rel)
		if err != nil {
			return err
		}
	}

	return nil
}

// destroyReleaseWorkspace will call the DestroyWorkspace hook if there
// are any valid operations. This expects all operations of this type to
// already be destroyed and will error if they are not.
func (a *App) destroyReleaseWorkspace(ctx context.Context) error {
	log := a.logger

	// Get the last destroyed value.
	resp, err := a.client.ListReleases(ctx, &pb.ListReleasesRequest{
		Application:   a.ref,
		Workspace:     a.workspace,
		PhysicalState: pb.Operation_DESTROYED,
		Order: &pb.OperationOrder{
			Order: pb.OperationOrder_COMPLETE_TIME,
			Limit: 1,
		},
	})
	if err != nil {
		return nil
	}

	// If we have no opeartions, we don't call the hook.
	results := resp.Releases
	if len(results) == 0 {
		return nil
	}

	// Call the hook
	d, ok := a.Releaser.(component.WorkspaceDestroyer)
	if !ok || d.DestroyWorkspaceFunc() == nil {
		// Workspace deletion is optional.
		return nil
	}

	a.UI.Output("Destroying shared release resources...", terminal.WithHeaderStyle())
	_, err = a.callDynamicFunc(ctx,
		log,
		nil,
		d,
		d.DestroyWorkspaceFunc(),
		argNamedAny("release", results[0].Release),
	)
	return err
}

type releaseDestroyOperation struct {
	Release *pb.Release
}

func (op *releaseDestroyOperation) Init(app *App) (proto.Message, error) {
	return op.Release, nil
}

func (op *releaseDestroyOperation) Hooks(app *App) map[string][]*config.Hook {
	return nil
}

func (op *releaseDestroyOperation) Labels(app *App) map[string]string {
	return op.Release.Labels
}

func (op *releaseDestroyOperation) Upsert(
	ctx context.Context,
	client pb.WaypointClient,
	msg proto.Message,
) (proto.Message, error) {
	d := msg.(*pb.Release)
	d.State = pb.Operation_DESTROYED

	resp, err := client.UpsertRelease(ctx, &pb.UpsertReleaseRequest{
		Release: d,
	})
	if err != nil {
		return nil, err
	}

	return resp.Release, nil
}

func (op *releaseDestroyOperation) Do(ctx context.Context, log hclog.Logger, app *App, _ proto.Message) (interface{}, error) {
	// If we have no releaser then we're done.
	if app.Releaser == nil {
		return nil, nil
	}

	destroyer, ok := app.Releaser.(component.Destroyer)

	// If we don't implement the destroy plugin we just mark it as destroyed.
	if !ok || destroyer.DestroyFunc() == nil {
		return nil, nil
	}

	if op.Release.Release == nil {
		log.Error("Unable to destroy the Release as the proto message Release returned from the plugin's ReleaseFunc is nil. This situation occurs when the release process is interupted by the user.", "release", op.Release)
		return nil, nil // Fail silently for now, this will be fixed in v0.2
	}

	return app.callDynamicFunc(ctx,
		log,
		nil,
		destroyer,
		destroyer.DestroyFunc(),
		argNamedAny("release", op.Release.Release),
	)
}

func (op *releaseDestroyOperation) StatusPtr(msg proto.Message) **pb.Status {
	return nil
}

func (op *releaseDestroyOperation) ValuePtr(msg proto.Message) **any.Any {
	return nil
}

var _ operation = (*releaseDestroyOperation)(nil)
