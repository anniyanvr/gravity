package rollingupdate

import (
	"fmt"

	"github.com/gravitational/gravity/lib/loc"
	"github.com/gravitational/gravity/lib/ops"
	"github.com/gravitational/gravity/lib/schema"
	"github.com/gravitational/gravity/lib/storage"
	"github.com/gravitational/gravity/lib/update/internal/builder"
	libphase "github.com/gravitational/gravity/lib/update/internal/rollingupdate/phases"

	"github.com/gravitational/trace"
)

// RuntimeConfigUpdates computes the runtime configuration updates for the specified list of servers
func RuntimeConfigUpdates(
	manifest schema.Manifest,
	operator ConfigPackageRotator,
	operationKey ops.SiteOperationKey,
	servers []storage.Server,
) (updates []storage.UpdateServer, err error) {
	updates = make([]storage.UpdateServer, 0, len(servers))
	for _, server := range servers {
		runtimePackage, err := manifest.RuntimePackageForProfile(server.Role)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		configUpdate, err := operator.RotatePlanetConfig(ops.RotatePlanetConfigRequest{
			Key:            operationKey,
			Server:         server,
			Manifest:       manifest,
			RuntimePackage: *runtimePackage,
			DryRun:         true,
		})
		if err != nil {
			return nil, trace.Wrap(err)
		}
		updates = append(updates, storage.UpdateServer{
			Server: server,
			Runtime: storage.RuntimePackage{
				Installed: *runtimePackage,
				Update: &storage.RuntimeUpdate{
					Package:       *runtimePackage,
					ConfigPackage: configUpdate.Locator,
				},
			},
		})
	}
	return updates, nil
}

// RuntimeConfigUpdatesWithSecrets computes the runtime configuration updates for the specified list of servers.
// The result includes updates for the secrets packages.
func RuntimeConfigUpdatesWithSecrets(
	manifest schema.Manifest,
	operator ConfigPackageRotator,
	operationKey ops.SiteOperationKey,
	servers []storage.Server,
) (updates []storage.UpdateServer, err error) {
	updates = make([]storage.UpdateServer, 0, len(servers))
	for _, server := range servers {
		runtimePackage, err := manifest.RuntimePackageForProfile(server.Role)
		if err != nil {
			return nil, trace.Wrap(err)
		}
		secretsUpdate, err := operator.RotateSecrets(ops.RotateSecretsRequest{
			Key:            operationKey.SiteKey(),
			Server:         server,
			RuntimePackage: *runtimePackage,
			DryRun:         true,
		})
		if err != nil {
			return nil, trace.Wrap(err)
		}
		configUpdate, err := operator.RotatePlanetConfig(ops.RotatePlanetConfigRequest{
			Key:            operationKey,
			Server:         server,
			Manifest:       manifest,
			RuntimePackage: *runtimePackage,
			DryRun:         true,
		})
		if err != nil {
			return nil, trace.Wrap(err)
		}
		updates = append(updates, storage.UpdateServer{
			Server: server,
			Runtime: storage.RuntimePackage{
				Installed:      *runtimePackage,
				SecretsPackage: &secretsUpdate.Locator,
				Update: &storage.RuntimeUpdate{
					Package:       *runtimePackage,
					ConfigPackage: configUpdate.Locator,
				},
			},
		})
	}
	return updates, nil
}

// ConfigPackageRotator defines the subset of Operator for updating package configuration
type ConfigPackageRotator interface {
	RotatePlanetConfig(ops.RotatePlanetConfigRequest) (*ops.RotatePackageResponse, error)
	RotateSecrets(ops.RotateSecretsRequest) (*ops.RotatePackageResponse, error)
}

// Config creates a new phase to update runtime container configuration
func (r Builder) Config(rootText string, servers []storage.UpdateServer) *builder.Phase {
	phase := storage.OperationPhase{
		ID:          "update-config",
		Executor:    libphase.UpdateConfig,
		Description: rootText,
		Data: &storage.OperationPhaseData{
			Package: &r.App,
		},
	}
	if len(servers) != 0 {
		phase.Data.Update = &storage.UpdateOperationData{
			Servers: servers,
		}
	}
	return builder.NewPhase(phase)
}

// Masters returns a new phase to execute a rolling update of the specified list of master servers
func (r Builder) Masters(servers []storage.UpdateServer, rootText, nodeTextFormat string) *builder.Phase {
	root := builder.NewPhase(storage.OperationPhase{
		ID:          "masters",
		Description: rootText,
	})
	first, others := servers[0], servers[1:]

	node := builder.NewPhase(r.node(first.Hostname, nodeTextFormat, first.Hostname))
	if len(others) != 0 {
		node.AddSequential(setLeaderElection(enable(), disable(first), first,
			"stepdown", "Step down %q as Kubernetes leader"))
	}
	node.AddSequential(r.commonFirstMaster(first, others...)...)
	root.AddSequential(node)
	for i, server := range others {
		node := builder.NewPhase(r.node(server.Hostname, nodeTextFormat, server.Hostname))
		node.AddSequential(r.common(others[i], nil)...)
		node.AddSequential(setLeaderElection(enable(server), disable(), server,
			"enable-elections", "Enable leader election on node %q"))
		root.AddSequential(node)
	}
	return root
}

// Nodes returns a new phase to execute a rolling update of the specified list of regular servers
func (r Builder) Nodes(servers []storage.UpdateServer, master storage.Server, rootText, nodeTextFormat string) *builder.Phase {
	root := builder.NewPhase(storage.OperationPhase{
		ID:          "nodes",
		Description: rootText,
	})
	for i, server := range servers {
		node := builder.NewPhase(r.node(server.Hostname, nodeTextFormat, server.Hostname))
		node.AddSequential(r.common(servers[i], &master)...)
		root.AddSequential(node)
	}
	return root
}

func (r Builder) commonFirstMaster(server storage.UpdateServer, others ...storage.UpdateServer) (phases []*builder.Phase) {
	phases = []*builder.Phase{
		r.drain(&server.Server, nil),
		r.restart(server),
	}
	if len(others) != 0 {
		phases = append(phases, setLeaderElection(enable(server), disable(others...), server,
			"elect", "Make node %q Kubernetes leader"))
	}
	if r.CustomUpdate != nil {
		return append(phases,
			r.custom(&server.Server),
			r.taint(&server.Server, nil),
			r.uncordon(&server.Server, nil),
			r.endpoints(&server.Server, nil),
			r.untaint(&server.Server, nil),
		)
	}
	return append(phases,
		r.taint(&server.Server, nil),
		r.uncordon(&server.Server, nil),
		r.endpoints(&server.Server, nil),
		r.untaint(&server.Server, nil),
	)
}

func (r Builder) common(server storage.UpdateServer, master *storage.Server) (phases []*builder.Phase) {
	return []*builder.Phase{
		r.drain(&server.Server, master),
		r.restart(server),
		r.taint(&server.Server, master),
		r.uncordon(&server.Server, master),
		r.endpoints(&server.Server, master),
		r.untaint(&server.Server, master),
	}
}

func (r Builder) restart(server storage.UpdateServer) *builder.Phase {
	node := r.node("restart", "Restart container on node %q", server.Hostname)
	node.Executor = libphase.RestartContainer
	node.Data = &storage.OperationPhaseData{
		ExecServer: &server.Server,
		Package:    &r.App,
		Update: &storage.UpdateOperationData{
			Servers: []storage.UpdateServer{server},
		},
	}
	return builder.NewPhase(node)
}

func (r Builder) taint(server, execer *storage.Server) *builder.Phase {
	node := r.node("taint", "Taint node %q", server.Hostname)
	node.Executor = libphase.Taint
	node.Data = &storage.OperationPhaseData{
		Server: server,
	}
	if execer != nil {
		node.Data.ExecServer = execer
	}
	return builder.NewPhase(node)
}

// custom assumes r.CustomUpdate != nil
func (r Builder) custom(server *storage.Server) *builder.Phase {
	node := *r.CustomUpdate
	node.Data.Server = server
	return builder.NewPhase(node)
}

func (r Builder) untaint(server, execer *storage.Server) *builder.Phase {
	node := r.node("untaint", "Remove taint from node %q", server.Hostname)
	node.Executor = libphase.Untaint
	node.Data = &storage.OperationPhaseData{
		Server: server,
	}
	if execer != nil {
		node.Data.ExecServer = execer
	}
	return builder.NewPhase(node)
}

func (r Builder) uncordon(server, execer *storage.Server) *builder.Phase {
	node := r.node("uncordon", "Uncordon node %q", server.Hostname)
	node.Executor = libphase.Uncordon
	node.Data = &storage.OperationPhaseData{
		Server: server,
	}
	if execer != nil {
		node.Data.ExecServer = execer
	}
	return builder.NewPhase(node)
}

func (r Builder) endpoints(server, execer *storage.Server) *builder.Phase {
	node := r.node("endpoints", "Wait for endpoints on node %q", server.Hostname)
	node.Executor = libphase.Endpoints
	node.Data = &storage.OperationPhaseData{
		Server: server,
	}
	if execer != nil {
		node.Data.ExecServer = execer
	}
	return builder.NewPhase(node)
}

func (r Builder) drain(server, execer *storage.Server) *builder.Phase {
	node := r.node("drain", "Drain node %q", server.Hostname)
	node.Executor = libphase.Drain
	node.Data = &storage.OperationPhaseData{
		Server: server,
	}
	if execer != nil {
		node.Data.ExecServer = execer
	}
	return builder.NewPhase(node)
}

func (r Builder) node(id, format string, args ...interface{}) storage.OperationPhase {
	return storage.OperationPhase{
		ID:          id,
		Description: fmt.Sprintf(format, args...),
	}
}

// Builder builds an operation plan
type Builder struct {
	// App specifies the cluster application
	App loc.Locator
	// CustomUpdate optionally specifies the custom phase
	CustomUpdate *storage.OperationPhase
}

// setLeaderElection creates a phase that will change the leader election state in the cluster
// enable - the list of servers to enable election on
// disable - the list of servers to disable election on
// server - The server the phase should be executed on, and used to name the phase
// key - is the identifier of the phase (combined with server.Hostname)
// msg - is a format string used to describe the phase
func setLeaderElection(enable, disable []storage.Server, server storage.UpdateServer, id, format string) *builder.Phase {
	return builder.NewPhase(storage.OperationPhase{
		ID:          id,
		Executor:    libphase.Elections,
		Description: fmt.Sprintf(format, server.Hostname),
		Data: &storage.OperationPhaseData{
			Server: &server.Server,
			ElectionChange: &storage.ElectionChange{
				EnableServers:  enable,
				DisableServers: disable,
			},
		},
	})
}

func servers(updates ...storage.UpdateServer) (result []storage.Server) {
	for _, update := range updates {
		result = append(result, update.Server)
	}
	return result
}

var disable = servers
var enable = servers
