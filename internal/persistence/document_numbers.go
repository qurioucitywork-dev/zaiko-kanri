package persistence

import (
 "errors"
 "fmt"
 "regexp"
 "time"
 "gorm.io/gorm"
)

var ErrDailyDocumentLimit = errors.New("同じ日・同じ種類の伝票は999件までです")
var documentPrefixPattern = regexp.MustCompile(`^[A-Z]+$`)

func formatDocumentNumber(prefix string, date time.Time, sequence int) string {
 return fmt.Sprintf("%s-%s%03d",prefix,date.Format("020106"),sequence)
}

// Allocated inside the document transaction: concurrent saves cannot reuse a
// number, rollbacks consume none, and archived documents keep their number.
func nextDailyDocumentNumber(tx *gorm.DB, org,prefix string, date,now time.Time) (string,error) {
 if !documentPrefixPattern.MatchString(prefix) || date.IsZero(){return "",errors.New("invalid document date or prefix")}
 var sequence int
 result:=tx.Raw(`INSERT INTO daily_document_sequences(organization_id,prefix,business_date,last_sequence,updated_at)
 VALUES(?,?,?,1,?) ON CONFLICT(organization_id,prefix,business_date)
 DO UPDATE SET last_sequence=daily_document_sequences.last_sequence+1,updated_at=EXCLUDED.updated_at
 WHERE daily_document_sequences.last_sequence<999 RETURNING last_sequence`,org,prefix,date.Format("2006-01-02"),now).Scan(&sequence)
 if result.Error!=nil{return "",result.Error}
 if sequence<1 || sequence>999{return "",ErrDailyDocumentLimit}
 return formatDocumentNumber(prefix,date,sequence),nil
}
