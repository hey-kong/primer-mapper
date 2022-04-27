package common

import (
	"regexp"
	"time"
)

const (
	// MemETPrefix the topic prefix for membership event
	MemETPrefix = "$hw/events/node/"
	// MemETUpdateSuffix the topic suffix for membership updated event
	MemETUpdateSuffix = "/membership/updated"
	// MemETDetailSuffix the topic suffix for membership detail
	MemETDetailSuffix = "/membership/detail"
	// MemETDetailResultSuffix the topic suffix for membership detail event
	MemETDetailResultSuffix = "/membership/detail/result"
	// MemETGetSuffix the topic suffix for membership get
	MemETGetSuffix = "/membership/get"
	// MemETGetResultSuffix the topic suffix for membership get event
	MemETGetResultSuffix = "/membership/get/result"

	// DeviceETPrefix the topic prefix for device event
	DeviceETPrefix = "$hw/events/device/"
	// TwinETUpdateSuffix the topic suffix for twin update event
	TwinETUpdateSuffix = "/twin/update"
	// TwinETUpdateResultSuffix the topic suffix for twin update result event
	TwinETUpdateResultSuffix = "/twin/update/result"
	// TwinETGetSuffix the topic suffix for twin get
	TwinETGetSuffix = "/twin/get"
	// TwinETGetResultSuffix the topic suffix for twin get event
	TwinETGetResultSuffix = "/twin/get/result"
	// TwinETCloudSyncSuffix the topic suffix for twin sync event
	TwinETCloudSyncSuffix = "/twin/cloud_updated"
	// TwinETEdgeSyncSuffix the topic suffix for twin sync event
	TwinETEdgeSyncSuffix = "/twin/edge_updated"
	// TwinETDeltaSuffix the topic suffix for twin delta event
	TwinETDeltaSuffix = "/twin/update/delta"
	// TwinETDocumentSuffix the topic suffix for twin document event
	TwinETDocumentSuffix = "/twin/update/document"

	// DeviceETUpdatedSuffix the topic suffix for device updated event
	DeviceETUpdatedSuffix = "/updated"
	// DeviceETStateUpdateSuffix the topic suffix for device state update event
	DeviceETStateUpdateSuffix = "/state/update"
	// DeviceETStateGetSuffix the topic suffix for device state get event
	DeviceETStateGetSuffix = "/state/get"
)

// Device the struct of device
type Device struct {
	ID          string              `json:"id,omitempty"`
	Name        string              `json:"name,omitempty"`
	Description string              `json:"description,omitempty"`
	State       string              `json:"state,omitempty"`
	LastOnline  string              `json:"last_online,omitempty"`
	Attributes  map[string]*MsgAttr `json:"attributes,omitempty"`
	Twin        map[string]*MsgTwin `json:"twin,omitempty"`
}

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

// MsgAttr the struct of device attr
type MsgAttr struct {
	Value    string        `json:"value"`
	Optional *bool         `json:"optional,omitempty"`
	Metadata *TypeMetadata `json:"metadata,omitempty"`
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

// DeviceTwinDelta the struct of device twin delta
type DeviceTwinDelta struct {
	BaseMessage
	Twin  map[string]*MsgTwin `json:"twin"`
	Delta map[string]string   `json:"delta"`
}

// MembershipDetail the struct of membership detail
type MembershipDetail struct {
	BaseMessage
	Devices []Device `json:"devices"`
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

//CreateBaseMessage function is used to create the base message
func CreateBaseMessage(timestamp int64) BaseMessage {
	var getMsg BaseMessage
	getMsg.Timestamp = timestamp
	return getMsg
}

//CreateActualUpdateMessage function is used to create the device twin update message
func CreateActualUpdateMessage(field string, value string, timestamp int64) DeviceTwinUpdate {
	var updateMsg DeviceTwinUpdate
	updateMsg.BaseMessage.Timestamp = timestamp
	updateMsg.Twin = map[string]*MsgTwin{}
	updateMsg.Twin[field] = &MsgTwin{}
	updateMsg.Twin[field].Actual = &TwinValue{Value: &value}
	return updateMsg
}
