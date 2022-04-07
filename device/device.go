package device

// ZigbeeDeviceState defines the zigbee device state values
type ZigbeeDeviceState struct {
	ID          string `json:"ID"`
	Temperature int    `json:"tem"`
	Humidity    int    `json:"hum"`
	Led         bool   `json:"led"`
}
