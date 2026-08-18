package service

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type parsedChannelTimePeriod struct {
	start      int
	end        int
	multiplier float64
}

type compiledChannelTimePricing struct {
	location *time.Location
	periods  []parsedChannelTimePeriod
	err      error
}

var (
	channelTimePricingLocations sync.Map
	channelTimePricingCompiled  sync.Map
)

// ValidateChannelTimePricing validates a channel's optional recurring pricing
// schedule. Empty/nil schedules intentionally mean disabled for compatibility.
func ValidateChannelTimePricing(config *ChannelTimePricing) error {
	if config == nil || len(config.Periods) == 0 {
		return nil
	}
	if _, err := loadChannelTimePricingLocation(config.Timezone); err != nil {
		return fmt.Errorf("timezone: %w", err)
	}
	_, err := parseChannelTimePeriods(config.Periods)
	return err
}

// MultiplierAt returns the configured multiplier for at. Invalid or stale
// persisted data fails open to 1.0; request billing must never be blocked by a
// display-only pricing schedule. Compiled schedules are cached by their value
// signature, keeping the billing hot path O(1) after the first use.
func (config *ChannelTimePricing) MultiplierAt(at time.Time) float64 {
	if config == nil || len(config.Periods) == 0 || at.IsZero() {
		return 1
	}
	compiled := compileChannelTimePricing(config)
	if compiled.err != nil || compiled.location == nil {
		return 1
	}
	local := at.In(compiled.location)
	second := local.Hour()*3600 + local.Minute()*60 + local.Second()
	for _, period := range compiled.periods {
		if second >= period.start && second < period.end {
			return period.multiplier
		}
	}
	return 1
}

func compileChannelTimePricing(config *ChannelTimePricing) *compiledChannelTimePricing {
	key := channelTimePricingKey(config)
	if cached, ok := channelTimePricingCompiled.Load(key); ok {
		return cached.(*compiledChannelTimePricing)
	}
	compiled := &compiledChannelTimePricing{}
	compiled.location, compiled.err = loadChannelTimePricingLocation(config.Timezone)
	if compiled.err == nil {
		compiled.periods, compiled.err = parseChannelTimePeriods(config.Periods)
	}
	actual, _ := channelTimePricingCompiled.LoadOrStore(key, compiled)
	return actual.(*compiledChannelTimePricing)
}

func channelTimePricingKey(config *ChannelTimePricing) string {
	if config == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(strings.TrimSpace(config.Timezone))
	for _, period := range config.Periods {
		b.WriteByte('|')
		b.WriteString(period.StartTime)
		b.WriteByte(',')
		b.WriteString(period.EndTime)
		b.WriteByte(',')
		b.WriteString(strconv.FormatFloat(period.Multiplier, 'g', -1, 64))
	}
	return b.String()
}

func loadChannelTimePricingLocation(name string) (*time.Location, error) {
	name = strings.TrimSpace(name)
	if name == "" || name == "Local" {
		return nil, fmt.Errorf("timezone must be an IANA timezone")
	}
	if cached, ok := channelTimePricingLocations.Load(name); ok {
		if location, valid := cached.(*time.Location); valid && location != nil {
			return location, nil
		}
		channelTimePricingLocations.Delete(name)
	}
	location, err := time.LoadLocation(name)
	if err != nil {
		return nil, err
	}
	actual, _ := channelTimePricingLocations.LoadOrStore(name, location)
	return actual.(*time.Location), nil
}

func parseChannelTime(value string, end bool) (int, error) {
	value = strings.TrimSpace(value)
	if end && (value == "00:00" || value == "00:00:00") {
		return 24 * 3600, nil
	}
	layout := "15:04:05"
	if len(value) == len("15:04") {
		layout = "15:04"
	}
	parsed, err := time.Parse(layout, value)
	if err != nil || parsed.Format(layout) != value {
		return 0, fmt.Errorf("time %q must use HH:mm or HH:mm:ss format", value)
	}
	return parsed.Hour()*3600 + parsed.Minute()*60 + parsed.Second(), nil
}

func parseChannelTimePeriods(periods []ChannelTimePricingPeriod) ([]parsedChannelTimePeriod, error) {
	if len(periods) > 96 {
		return nil, fmt.Errorf("at most 96 time pricing periods are allowed")
	}
	parsed := make([]parsedChannelTimePeriod, 0, len(periods))
	for _, period := range periods {
		if math.IsNaN(period.Multiplier) || math.IsInf(period.Multiplier, 0) || period.Multiplier < 0.01 {
			return nil, fmt.Errorf("multiplier must be finite and at least 0.01")
		}
		scaled := period.Multiplier * 100
		if math.IsNaN(scaled) || math.IsInf(scaled, 0) || math.Abs(scaled-math.Round(scaled)) > 1e-9 {
			return nil, fmt.Errorf("multiplier must have at most two decimal places")
		}
		start, err := parseChannelTime(period.StartTime, false)
		if err != nil {
			return nil, err
		}
		end, err := parseChannelTime(period.EndTime, true)
		if err != nil {
			return nil, err
		}
		if start >= end {
			return nil, fmt.Errorf("start time must be before end time")
		}
		parsed = append(parsed, parsedChannelTimePeriod{start: start, end: end, multiplier: period.Multiplier})
	}
	sort.Slice(parsed, func(i, j int) bool { return parsed[i].start < parsed[j].start })
	for i := 1; i < len(parsed); i++ {
		if parsed[i].start < parsed[i-1].end {
			return nil, fmt.Errorf("time pricing periods overlap")
		}
	}
	return parsed, nil
}
