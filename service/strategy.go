package service

import (
	"encoding/json"

	"github.com/pkg/errors"
)

type CheckResult struct {
	checker Checker
	result  bool
}

type Strategy func([]CheckResult) (bool, error)

func All() Strategy {
	return func(results []CheckResult) (bool, error) {
		failed := 0
		for _, result := range results {
			if !result.result {
				failed += 1
			}
		}
		return failed == len(results), nil
	}
}

func AtLeastOne() Strategy {
	return func(results []CheckResult) (bool, error) {
		for _, result := range results {
			if !result.result {
				return true, nil
			}
		}
		return false, nil
	}
}

type GroupData struct {
	total  int
	failed int
}

func AllInGroup() Strategy {
	return func(results []CheckResult) (bool, error) {
		groups := make(map[string]GroupData)

		for _, result := range results {
			var groupData GroupData
			var ok bool

			if groupData, ok = groups[result.checker.Group]; !ok {
				groupData = GroupData{}
			}
			groupData.total += 1
			if !result.result {
				groupData.failed += 1
			}

			groups[result.checker.Group] = groupData
		}
		for _, groupData := range groups {
			if groupData.total == groupData.failed {
				return true, nil
			}
		}
		return false, nil
	}
}

type AtLeastNPercentageParams struct {
	N float64 `json:"n"`
}

func AtLeastNPercentage(options json.RawMessage) (Strategy, error) {
	params := AtLeastNPercentageParams{}
	if err := json.Unmarshal(options, &params); err != nil {
		return nil, err
	}

	return func(results []CheckResult) (bool, error) {
		failed := 0.0
		for _, result := range results {
			if !result.result {
				failed += 1
			}
		}
		return failed/float64(len(results)) > params.N, nil
	}, nil
}

func GetStrategy(strategyName string, strategyOptions json.RawMessage) (Strategy, error) {
	if strategyName == "" {
		strategyName = "at_least_one"
	}
	switch strategyName {
	case "all":
		return All(), nil
	case "at_least_one":
		return AtLeastOne(), nil
	case "all_in_group":
		return AllInGroup(), nil
	case "at_least_n_percentage":
		return AtLeastNPercentage(strategyOptions)
	default:
		return nil, errors.Errorf("unknown strategy %s", strategyName)
	}
}

func GetStrategyNoOptions(strategyName string) (Strategy, error) {
	return GetStrategy(strategyName, json.RawMessage(``))
}
