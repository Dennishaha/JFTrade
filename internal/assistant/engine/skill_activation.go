package adk

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	adksession "google.golang.org/adk/v2/session"
)

const skillActivationStateKeyPrefix = adksession.KeyPrefixTemp + "jftrade.skill_activation."

func skillActivationStateKey(agentName string, skillName string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(agentName) + "\x00" + strings.TrimSpace(skillName)))
	return skillActivationStateKeyPrefix + hex.EncodeToString(sum[:])
}

func activateSkill(state adksession.State, agentName string, skillName string) error {
	agentName = strings.TrimSpace(agentName)
	skillName = strings.TrimSpace(skillName)
	if state == nil || agentName == "" || skillName == "" {
		return fmt.Errorf("skill activation requires state, agent name and skill name")
	}
	return state.Set(skillActivationStateKey(agentName, skillName), true)
}

func skillActiveInState(state adksession.ReadonlyState, agentName string, skillName string) bool {
	agentName = strings.TrimSpace(agentName)
	skillName = strings.TrimSpace(skillName)
	if state == nil || agentName == "" || skillName == "" {
		return false
	}
	value, err := state.Get(skillActivationStateKey(agentName, skillName))
	if err != nil {
		return false
	}
	active, ok := value.(bool)
	return ok && active
}

func anySkillActiveInState(state adksession.ReadonlyState, agentName string, skillNames []string) bool {
	for _, skillName := range normalizeStringSlice(skillNames) {
		if skillActiveInState(state, agentName, skillName) {
			return true
		}
	}
	return false
}
