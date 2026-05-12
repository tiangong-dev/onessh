package ports

// AgentConfig carries passcache agent connection settings across app boundaries.
type AgentConfig struct {
	Socket     string
	Capability string
}
