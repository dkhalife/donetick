package notifier

import (
	"context"
	"time"

	nModel "donetick.com/core/internal/models/notifier"
	"gorm.io/gorm"
)

type NotificationRepository struct {
	db *gorm.DB
}

func NewNotificationRepository(db *gorm.DB) *NotificationRepository {
	return &NotificationRepository{db}
}

func (r *NotificationRepository) DeleteAllTaskNotifications(taskID int) error {
	return r.db.Where("task_id = ?", taskID).Delete(&nModel.Notification{}).Error
}

func (r *NotificationRepository) BatchInsertNotifications(notifications []*nModel.Notification) error {
	return r.db.Create(&notifications).Error
}
func (r *NotificationRepository) MarkNotificationsAsSent(notifications []*nModel.Notification) error {
	var ids []int
	for _, notification := range notifications {
		ids = append(ids, notification.ID)
	}

	return r.db.Model(&nModel.Notification{}).Where("id IN (?)", ids).Update("is_sent", true).Error
}
func (r *NotificationRepository) GetPendingNotificaiton(c context.Context, lookback time.Duration) ([]*nModel.Notification, error) {
	var notifications []*nModel.Notification
	start := time.Now().UTC().Add(-lookback)
	end := time.Now().UTC()
	if err := r.db.Where("is_sent = ? AND scheduled_for < ? AND scheduled_for > ?", false, end, start).Find(&notifications).Error; err != nil {
		return nil, err
	}
	return notifications, nil
}

func (r *NotificationRepository) DeleteSentNotifications(c context.Context, since time.Time) error {
	return r.db.WithContext(c).Where("is_sent = ? AND scheduled_for < ?", true, since).Delete(&nModel.Notification{}).Error
}
