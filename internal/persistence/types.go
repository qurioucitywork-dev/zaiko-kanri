package persistence

import (
	"database/sql/driver"
	"fmt"
	"time"
)

// DateString keeps the REST contract as YYYY-MM-DD while PostgreSQL stores a
// native DATE value.
type DateString string

func (d *DateString) Scan(value any) error {
	switch typed := value.(type) {
	case nil:
		*d = ""
	case time.Time:
		*d = DateString(typed.Format("2006-01-02"))
	case string:
		*d = DateString(typed)
	case []byte:
		*d = DateString(string(typed))
	default:
		return fmt.Errorf("unsupported date value %T", value)
	}
	return nil
}

func (d DateString) Value() (driver.Value, error) {
	if d == "" {
		return nil, nil
	}
	return string(d), nil
}

func (DateString) GormDataType() string { return "date" }
