// Package platform_settings wires the operator-configurable platform
// settings module: a key-value store with a 30s TTL cache, a registry
// of known keys with defaults, and an admin CRUD handler.
package platform_settings

import (
	"github.com/hangrix/hangrix/apps/hangrix/internal/modules/platform_settings/domain"
	"github.com/hangrix/hangrix/apps/hangrix/internal/modules/platform_settings/handler"
	"github.com/hangrix/hangrix/apps/hangrix/internal/modules/platform_settings/infra"
	"github.com/hangrix/hangrix/apps/hangrix/internal/modules/platform_settings/service"
	"github.com/hangrix/hangrix/apps/hangrix/internal/server"
	"github.com/hangrix/hangrix/pkg/ioc"
)

// Known lifecycle keys with their defaults.
var lifecycleDefinitions = []domain.Definition{
	{
		Key:         "lifecycle.idle_stop_threshold",
		Default:     "1h",
		Description: "How long an idle session waits before the platform flags its container for docker stop",
		Type:        domain.ValueTypeDuration,
	},
	{
		Key:         "lifecycle.idle_removal_threshold",
		Default:     "168h",
		Description: "How long after a container is flagged for cleanup before the runner's reaper docker rm's it (7 days)",
		Type:        domain.ValueTypeDuration,
	},
	{
		Key:         "lifecycle.abandoned_cleanup_threshold",
		Default:     "720h",
		Description: "How long after a container is flagged for cleanup with no runner pickup before the platform gives up and clears the container_id (30 days)",
		Type:        domain.ValueTypeDuration,
	},
	{
		Key:         domain.SettingFirepowerEnabled,
		Default:     "false",
		Description: "Temporary global firepower mode. When enabled, selected agent roles prefer gpt-5.5 and runner task polling can use the elevated firepower batch limit.",
		Type:        domain.ValueTypeBool,
	},
	{
		Key:         domain.SettingRunnerMaxTasksPerPoll,
		Default:     "64",
		Description: "Default server-side maximum workflow tasks returned to one runner poll.",
		Type:        domain.ValueTypeInt,
	},
	{
		Key:         domain.SettingFirepowerRunnerMaxTasks,
		Default:     "256",
		Description: "Server-side maximum workflow tasks returned to one runner poll while firepower mode is enabled.",
		Type:        domain.ValueTypeInt,
	},
	{
		Key:         domain.SettingChatGPTFastMode,
		Default:     "false",
		Description: "When enabled, OpenAI-native ChatGPT providers are called through the /fast Responses API path.",
		Type:        domain.ValueTypeBool,
	},
}

func Module() *ioc.Module {
	m := ioc.NewModule()

	// Registry is a pure-data value; register it as a singleton.
	registry := domain.NewRegistry(lifecycleDefinitions)
	m.Provide(func() *domain.Registry { return registry }).ToSelf()

	// Persistence — bind to the narrow service.Repo interface the
	// service layer consumes. ioc matches deps by exact reflect.Type,
	// so the concrete *infra.PostgresRepo must be registered under the
	// interface type, not just ToSelf().
	m.Provide(infra.NewPostgresRepo).ToInterface(new(service.Repo))

	// Service: cache + Store interface. Bind to domain.Store so the
	// reaper and admin handlers consume it through the interface.
	svc := m.Provide(service.NewService)
	svc.ToInterface(new(domain.Store))

	// Admin handler
	m.Provide(handler.NewAdminHandler).ToInterface(new(server.RouteProvider))

	return m
}
