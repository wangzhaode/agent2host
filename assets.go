package agent2host

import _ "embed"

//go:embed skills/agent2host/SKILL.md
var agentSkill []byte

// AgentSkill returns a copy of the bundled Agent2Host Skill.
func AgentSkill() []byte {
	return append([]byte(nil), agentSkill...)
}
