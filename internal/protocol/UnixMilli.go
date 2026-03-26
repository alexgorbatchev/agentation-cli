package protocol

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type UnixMilli int64

func (u UnixMilli) MarshalJSON() ([]byte, error) {
	return json.Marshal(int64(u))
}

func (u *UnixMilli) UnmarshalJSON(data []byte) error {
	parsed, error := parseUnixMilliJSON(data)
	if error != nil {
		return error
	}
	*u = parsed
	return nil
}

func (u *UnixMilli) Scan(value any) error {
	parsed, error := parseUnixMilliValue(value)
	if error != nil {
		return error
	}
	*u = parsed
	return nil
}

func (u UnixMilli) Value() (driver.Value, error) {
	return int64(u), nil
}

func (u UnixMilli) Time() time.Time {
	if u == 0 {
		return time.Time{}
	}
	return time.UnixMilli(int64(u)).UTC()
}

func ParseUnixMilli(value string) (UnixMilli, error) {
	trimmedValue := strings.TrimSpace(value)
	if trimmedValue == "" {
		return 0, nil
	}

	parsedInt, error := strconv.ParseInt(trimmedValue, 10, 64)
	if error != nil {
		return 0, fmt.Errorf("invalid timestamp %q", value)
	}
	return UnixMilli(parsedInt), nil
}

func parseUnixMilliJSON(data []byte) (UnixMilli, error) {
	trimmedData := strings.TrimSpace(string(data))
	if trimmedData == "" || trimmedData == "null" {
		return 0, nil
	}

	var parsedInt int64
	if error := json.Unmarshal(data, &parsedInt); error == nil {
		return UnixMilli(parsedInt), nil
	}

	var parsedString string
	if error := json.Unmarshal(data, &parsedString); error == nil {
		return ParseUnixMilli(parsedString)
	}

	return 0, fmt.Errorf("invalid timestamp payload %s", trimmedData)
}

func parseUnixMilliValue(value any) (UnixMilli, error) {
	switch typedValue := value.(type) {
	case nil:
		return 0, nil
	case int64:
		return UnixMilli(typedValue), nil
	case int32:
		return UnixMilli(typedValue), nil
	case int:
		return UnixMilli(typedValue), nil
	case float64:
		return UnixMilli(int64(typedValue)), nil
	case []byte:
		return ParseUnixMilli(string(typedValue))
	case string:
		return ParseUnixMilli(typedValue)
	case time.Time:
		return UnixMilli(typedValue.UTC().UnixMilli()), nil
	default:
		return 0, fmt.Errorf("unsupported timestamp value type %T", value)
	}
}
