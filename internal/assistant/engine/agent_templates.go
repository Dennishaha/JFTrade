package adk

const DefaultBuiltinAgentID = "jftrade-default"

func BuiltinAgentTemplates() []AgentWriteRequest {
	return []AgentWriteRequest{
		{
			ID: DefaultBuiltinAgentID, Name: "默认助手",
			Instruction:    defaultAgentInstruction(),
			PermissionMode: PermissionModeApproval, Status: AgentStatusEnabled, MemoryEnabled: true, WorkMode: WorkModeChat, LoopMaxIterations: DefaultLoopMaxIterations,
			Tools:  nil,
			Skills: BuiltinSkillIDs(),
		},
	}
}

func BuiltinAgentTemplate(id string) (AgentWriteRequest, bool) {
	id = normalizeID(id)
	for _, template := range BuiltinAgentTemplates() {
		if normalizeID(template.ID) == id {
			return template, true
		}
	}
	return AgentWriteRequest{}, false
}

func IsBuiltinAgentID(id string) bool {
	_, ok := BuiltinAgentTemplate(id)
	return ok
}

func IsPrimaryBuiltinAgentID(id string) bool {
	return normalizeID(id) == DefaultBuiltinAgentID
}
