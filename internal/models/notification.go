// notification.go — Notification model: extensible notification system for user-facing events.
//
// DESIGN:
//
//	Notifications are a flexible, type-driven system supporting multiple event
//	categories (admin invites, upvote milestones, etc.). Each notification has a
//	type, a target user, optional action metadata, and read/actioned state.
//
//	Admin invite notifications carry an actionable payload (accept/deny) that
//	modifies subnotery admin membership. Milestone notifications are informational
//	and link to the relevant content.
//
// TYPES:
//   - admin_invite:       Invitation to become a subnotery admin (actionable: accept/deny)
//   - upvote_milestone:   Post or comment reached an upvote milestone (informational)
//   - purchase:           Someone purchased your note (informational)
//   - comment:            Someone commented on your note (informational)
//   - reply:              Someone replied to your comment (informational)
//
// STORAGE:
//
//	All notifications live in a single polymorphic table. The `type` column
//	determines how `reference_id` and `metadata` are interpreted:
//	  - admin_invite:     reference_id = subnotery_id, metadata = {"invited_by": <user_id>}
//	  - upvote_milestone: reference_id = note_id or comment_id, metadata = {"milestone": 10, "target": "note"|"comment"}
//	  - purchase:         reference_id = note_id, metadata = {"buyer_id": <user_id>}
//	  - comment:          reference_id = note_id, metadata = {"comment_id": <id>}
//	  - reply:            reference_id = comment_id (parent), metadata = {"reply_id": <id>}
package models

import "time"

// NotificationType categorises the notification for routing and display.
type NotificationType string

const (
	// NotifAdminInvite is an actionable invitation to become a subnotery admin.
	NotifAdminInvite NotificationType = "admin_invite"
	// NotifUpvoteMilestone is an informational notification for upvote milestones.
	NotifUpvoteMilestone NotificationType = "upvote_milestone"
	// NotifPurchase is an informational notification sent to note owners on purchase.
	NotifPurchase NotificationType = "purchase"
	// NotifComment is an informational notification sent to note owners on new comments.
	NotifComment NotificationType = "comment"
	// NotifReply is an informational notification sent when someone replies to a comment.
	NotifReply NotificationType = "reply"
)

// NotificationStatus tracks whether an actionable notification has been resolved.
type NotificationStatus string

const (
	// NotifPending means the notification has not been acted on yet.
	NotifPending NotificationStatus = "pending"
	// NotifAccepted means the user accepted (e.g., accepted admin invite).
	NotifAccepted NotificationStatus = "accepted"
	// NotifDenied means the user denied (e.g., declined admin invite).
	NotifDenied NotificationStatus = "denied"
)

// Notification represents a single notification delivered to a user.
//
// Fields:
//   - UserID:       The recipient of this notification
//   - Type:         Notification category (admin_invite, upvote_milestone, etc.)
//   - Title:        Short summary shown in the notification dropdown
//   - Message:      Longer description or context
//   - ReferenceID:  Polymorphic foreign key (subnotery_id, note_id, comment_id)
//   - ReferenceType: What ReferenceID points to ("subnotery", "note", "comment")
//   - ActionStatus: For actionable notifications (pending/accepted/denied)
//   - IsRead:       Whether the user has seen the notification
//   - ActorID:      The user who triggered the notification (e.g., admin who invited)
//   - Metadata:     JSON string for extensible type-specific data
type Notification struct {
	ID            uint               `json:"id" gorm:"primaryKey"`
	UserID        uint64             `json:"user_id" gorm:"index;not null"`
	Type          NotificationType   `json:"type" gorm:"type:varchar(50);not null;index"`
	Title         string             `json:"title" gorm:"type:varchar(255);not null"`
	Message       string             `json:"message" gorm:"type:text"`
	ReferenceID   uint64             `json:"reference_id" gorm:"index"`
	ReferenceType string             `json:"reference_type" gorm:"type:varchar(50)"`
	ActionStatus  NotificationStatus `json:"action_status" gorm:"type:varchar(20);default:'pending'"`
	IsRead        bool               `json:"is_read" gorm:"default:false;index"`
	ActorID       uint64             `json:"actor_id"`
	Metadata      string             `json:"metadata" gorm:"type:text"`
	CreatedAt     time.Time          `json:"created_at" gorm:"index"`
	UpdatedAt     time.Time          `json:"updated_at"`
}

// UpvoteMilestones defines the upvote thresholds that trigger notifications.
// When a post or comment reaches one of these counts, the author is notified.
var UpvoteMilestones = []uint64{5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000, 10000}

// ShouldNotifyMilestone checks whether the new upvote count has crossed a milestone.
// Returns the milestone value and true if a notification should fire, or 0 and false.
func ShouldNotifyMilestone(oldUpvotes, newUpvotes uint64) (uint64, bool) {
	for _, m := range UpvoteMilestones {
		if oldUpvotes < m && newUpvotes >= m {
			return m, true
		}
	}
	return 0, false
}
