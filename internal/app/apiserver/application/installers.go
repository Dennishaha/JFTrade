package application

import "fmt"

// Installers is a compile-time startup chain. Each installer receives only the
// bundle published by its immediate predecessor.
type Installers[Platform, MarketData, Trading, StrategyBacktest, AssistantHTTP any] struct {
	Platform         func() (Platform, error)
	MarketData       func(Platform) (MarketData, error)
	Trading          func(MarketData) (Trading, error)
	StrategyBacktest func(Trading) (StrategyBacktest, error)
	AssistantHTTP    func(StrategyBacktest) (AssistantHTTP, error)
	Validate         func() error
	Rollback         func(error) error
}

func (i Installers[Platform, MarketData, Trading, StrategyBacktest, AssistantHTTP]) Run() error {
	platform, err := i.Platform()
	err = i.validate(err)
	if err != nil {
		return i.fail("platform/store/calendar/observability", err)
	}
	marketData, err := i.MarketData(platform)
	err = i.validate(err)
	if err != nil {
		return i.fail("market data", err)
	}
	trading, err := i.Trading(marketData)
	err = i.validate(err)
	if err != nil {
		return i.fail("trading", err)
	}
	strategyBacktest, err := i.StrategyBacktest(trading)
	err = i.validate(err)
	if err != nil {
		return i.fail("strategy/backtest", err)
	}
	_, err = i.AssistantHTTP(strategyBacktest)
	err = i.validate(err)
	if err != nil {
		return i.fail("assistant/http", err)
	}
	return nil
}

func (i Installers[Platform, MarketData, Trading, StrategyBacktest, AssistantHTTP]) validate(err error) error {
	if err == nil && i.Validate != nil {
		return i.Validate()
	}
	return err
}

func (i Installers[Platform, MarketData, Trading, StrategyBacktest, AssistantHTTP]) fail(stage string, startupErr error) error {
	err := fmt.Errorf("install %s: %w", stage, startupErr)
	if i.Rollback != nil {
		return i.Rollback(err)
	}
	return err
}
