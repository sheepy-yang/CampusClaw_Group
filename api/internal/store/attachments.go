package store

import "strings"

func attachmentID(digest, classID, ownerID string) string {
	return digest + ":" + Digest(classID + "\x00" + ownerID)[:16]
}

func (m *Memory) SaveAttachment(attachment Attachment) error {
	if attachment.ID == "" {
		attachment.ID = attachmentID(attachment.Digest, attachment.ClassID, attachment.OwnerID)
	}
	if err := m.persistenceStore().save(
		"attachment", attachment.ID, attachment,
		attachment.ClassID, "owner:"+attachment.OwnerID, "digest:"+attachment.Digest,
	); err != nil {
		return err
	}
	m.mu.Lock()
	if m.Attachments == nil {
		m.Attachments = make(map[string]Attachment)
	}
	m.Attachments[attachment.ID] = attachment
	m.mu.Unlock()
	return nil
}

func (m *Memory) GetAttachment(digest, classID, ownerID string) (Attachment, bool) {
	digest = strings.TrimSpace(digest)
	id := attachmentID(digest, classID, ownerID)
	m.mu.RLock()
	attachment, ok := m.Attachments[id]
	m.mu.RUnlock()
	if ok {
		return attachment, true
	}
	if err := m.persistenceStore().load("attachment", id, &attachment); err != nil {
		return Attachment{}, false
	}
	m.mu.Lock()
	if m.Attachments == nil {
		m.Attachments = make(map[string]Attachment)
	}
	m.Attachments[id] = attachment
	m.mu.Unlock()
	return attachment, true
}
