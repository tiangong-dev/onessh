package domain

const DefaultSSHPort = 22

func EffectivePort(port int) int {
	if port <= 0 {
		return DefaultSSHPort
	}
	return port
}
