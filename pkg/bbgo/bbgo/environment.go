package bbgo

import (
	"context"
	"time"

	"github.com/jftrade/jftrade-main/pkg/bbgo/types"
)

type Environment struct {
	startTime time.Time
	sessions  map[string]*ExchangeSession
}

func NewEnvironment() *Environment {
	now := time.Now()
	return &Environment{
		startTime: now,
		sessions:  make(map[string]*ExchangeSession),
	}
}

func (e *Environment) SetStartTime(start time.Time) {
	e.startTime = start
}

func (e *Environment) AddExchange(name string, exchange types.Exchange) *ExchangeSession {
	session := NewExchangeSession(name, exchange)
	e.sessions[name] = session
	return session
}

func (e *Environment) Init(ctx context.Context) error {
	for _, session := range e.sessions {
		if session == nil || session.Exchange == nil {
			continue
		}
		account, err := session.Exchange.QueryAccount(ctx)
		if err == nil && account != nil {
			session.Account = account
		}
	}
	return nil
}
