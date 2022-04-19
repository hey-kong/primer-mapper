package common

import (
	"regexp"
	"time"
)

const (
	DevicePrefix            = "$hw/events/device/"
	TwinUpdateSuffix        = "/twin/update"
	TwinUpdateDeltaSuffix   = "/twin/update/delta"
	TwinUpdateResultSuffix  = "/twin/update/result"
	DeviceStateUpdateSuffix = "/state/update"
	TwinCloudSyncSuffix     = "/twin/cloud_updated"
	TwinGetResultSuffix     = "/twin/get/result"
	TwinGetSuffix           = "/twin/get"
)

// DeviceStateUpdate is the structure used in updating the device state
type DeviceStateUpdate struct {
	State string `json:"state,omitempty"`
}

// BaseMessage the base struct of event message
type BaseMessage struct {
	EventID   string `json:"event_id"`
	Timestamp int64  `json:"timestamp"`
}

// TwinValue the struct of twin value
type TwinValue struct {
	Value    *string        `json:"value,omitempty"`
	Metadata *ValueMetadata `json:"metadata,omitempty"`
}

// ValueMetadata the meta of value
type ValueMetadata struct {
	Timestamp int64 `json:"timestamp,omitempty"`
}

// TypeMetadata the meta of value type
type TypeMetadata struct {
	Type string `json:"type,omitempty"`
}

// TwinVersion twin version
type TwinVersion struct {
	CloudVersion int64 `json:"cloud"`
	EdgeVersion  int64 `json:"edge"`
}

// MsgTwin the struct of device twin
type MsgTwin struct {
	Expected        *TwinValue    `json:"expected,omitempty"`
	Actual          *TwinValue    `json:"actual,omitempty"`
	Optional        *bool         `json:"optional,omitempty"`
	Metadata        *TypeMetadata `json:"metadata,omitempty"`
	ExpectedVersion *TwinVersion  `json:"expected_version,omitempty"`
	ActualVersion   *TwinVersion  `json:"actual_version,omitempty"`
}

// DeviceTwinUpdate the struct of device twin update
type DeviceTwinUpdate struct {
	BaseMessage
	Twin map[string]*MsgTwin `json:"twin"`
}

// DeviceTwinDelta twin delta
type DeviceTwinDelta struct {
	BaseMessage
	Twin  map[string]*MsgTwin `json:"twin"`
	Delta map[string]string   `json:"delta"`
}

func GetTimestamp() int64 {
	return time.Now().UnixNano() / 1e6
}

func MatchType(str string) string {
	if matched, _ := regexp.Match("[-+]?[0-9]*\\.?[0-9]+f", []byte(str)); matched {
		return "float"
	} else if matched, _ := regexp.Match("[-+]?[0-9]+", []byte(str)); matched {
		return "int"
	} else {
		return "string"
	}
}
