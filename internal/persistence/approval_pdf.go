package persistence

import (
	"context"
	"encoding/json"
)

func (r *Repository) SaveApprovalPDF(ctx context.Context, org, id string, ref *OfficialDocumentRef) error {
	data, err := json.Marshal(ref)
	if err != nil {
		return err
	}
	return r.db.WithContext(ctx).Exec("UPDATE approval_requests SET snapshot_json=jsonb_set(snapshot_json,'{officialPDF}',CAST(? AS JSONB)) WHERE organization_id=? AND id=? AND status='approved'", string(data), org, id).Error
}
