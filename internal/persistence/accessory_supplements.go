package persistence

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"gorm.io/gorm"
	"regexp"
	"strconv"
	"strings"
)

var ErrAccessorySupplement = errors.New("invalid accessory supplement")

// Values are keyed by stable accessory master code, never by the display name.
type AccessorySupplementValues map[string]string

func (AccessorySupplementValues) GormDataType() string { return "text" }
func (v AccessorySupplementValues) Value() (driver.Value, error) {
	if v == nil {
		return "{}", nil
	}
	b, e := json.Marshal(v)
	return string(b), e
}
func (v *AccessorySupplementValues) Scan(src any) error {
	if src == nil {
		*v = AccessorySupplementValues{}
		return nil
	}
	var b []byte
	switch s := src.(type) {
	case string:
		b = []byte(s)
	case []byte:
		b = s
	default:
		return fmt.Errorf("invalid accessory value storage: %T", src)
	}
	if len(b) == 0 {
		*v = AccessorySupplementValues{}
		return nil
	}
	return json.Unmarshal(b, v)
}

var accessoryNumber = regexp.MustCompile(`^[0-9]{1,9}$`)

func normalizeAccessorySupplement(label, value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	label = strings.TrimSpace(label)
	if label == "フリーワード" {
		if len([]rune(value)) > 200 {
			return "", ErrAccessorySupplement
		}
		return value, nil
	}
	if strings.Contains(label, "年") && strings.Contains(label, "月") {
		parts := strings.Split(value, "/")
		if len(parts) != 2 {
			return "", ErrAccessorySupplement
		}
		year, e1 := strconv.Atoi(parts[0])
		month, e2 := strconv.Atoi(parts[1])
		if e1 != nil || e2 != nil || !accessoryNumber.MatchString(parts[0]) || !accessoryNumber.MatchString(parts[1]) || year < 1 || year > 9999 || month < 1 || month > 12 {
			return "", ErrAccessorySupplement
		}
		return fmt.Sprintf("%d/%d", year, month), nil
	}
	if !accessoryNumber.MatchString(value) {
		return "", ErrAccessorySupplement
	}
	n, _ := strconv.Atoi(value)
	if strings.Contains(label, "月") && (n < 1 || n > 12) {
		return "", ErrAccessorySupplement
	}
	return strconv.Itoa(n), nil
}
func normalizeAccessorySupplements(tx *gorm.DB, org string, codes []string, values AccessorySupplementValues) (AccessorySupplementValues, error) {
	result := AccessorySupplementValues{}
	if len(values) == 0 {
		return result, nil
	}
	var masters []MasterItem
	if err := tx.Table("accessories").Where("organization_id=? AND code IN ?", org, normalizeCodes(codes)).Find(&masters).Error; err != nil {
		return nil, err
	}
	for _, m := range masters {
		if m.Supplement == "" {
			continue
		}
		value, err := normalizeAccessorySupplement(m.Supplement, values[m.Code])
		if err != nil {
			return nil, fmt.Errorf("%w: %s", err, m.Name)
		}
		if value != "" {
			result[m.Code] = value
		}
	}
	return result, nil
}
func saveAccessorySupplementValues(tx *gorm.DB, table, id string, values AccessorySupplementValues) error {
	value, err := values.Value()
	if err != nil {
		return err
	}
	return tx.Table(table).Where("id=?", id).UpdateColumn("accessory_supplements", value).Error
}
