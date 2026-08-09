package adk

import (
	enginepersistence "github.com/jftrade/jftrade-main/internal/assistant/engine/persistence"
	jfadkmodel "github.com/jftrade/jftrade-main/internal/assistant/model"
)

var ErrChatRequestConflict = enginepersistence.ErrChatRequestConflict

type ChatRequestConflictError = enginepersistence.ChatRequestConflictError

type ReusedChatRequestError = jfadkmodel.ReusedChatRequestError

func ensureChatRequestIdentity(req ChatRequest) (ChatRequest, string, error) {
	return enginepersistence.EnsureChatRequestIdentity(req)
}

func NormalizeChatRequestIdentity(req ChatRequest) (ChatRequest, string, error) {
	return enginepersistence.NormalizeChatRequestIdentity(req)
}
