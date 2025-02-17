package repos

import (
	"context"
	"errors"

	config "dkhalife.com/tasks/core/config"
	lModel "dkhalife.com/tasks/core/internal/models/label"
	tModel "dkhalife.com/tasks/core/internal/models/task"
	"dkhalife.com/tasks/core/internal/services/logging"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type LabelRepository struct {
	db *gorm.DB
}

func NewLabelRepository(db *gorm.DB, cfg *config.Config) *LabelRepository {
	return &LabelRepository{db: db}
}

func (r *LabelRepository) GetUserLabels(ctx context.Context, userID int) ([]*lModel.Label, error) {
	var labels []*lModel.Label
	if err := r.db.WithContext(ctx).Select("id", "name", "color").Where("created_by = ?", userID).Find(&labels).Error; err != nil {
		return nil, err
	}
	return labels, nil
}

func (r *LabelRepository) CreateLabels(ctx context.Context, labels []*lModel.Label) error {
	if err := r.db.WithContext(ctx).Create(&labels).Error; err != nil {
		return err
	}
	return nil
}

func (r *LabelRepository) isLabelsAssignableByUser(ctx context.Context, userID int, toBeAdded []int, toBeRemoved []int) bool {
	labelIDs := append(toBeAdded, toBeRemoved...)

	log := logging.FromContext(ctx)
	var count int64
	if err := r.db.WithContext(ctx).Model(&lModel.Label{}).Where("id IN (?) AND created_by = ?", labelIDs, userID).Count(&count).Error; err != nil {
		log.Error(err)
		return false
	}
	return count == int64(len(labelIDs))
}

func (r *LabelRepository) AssignLabelsToTask(ctx context.Context, taskID int, userID int, toBeAdded []int, toBeRemoved []int) error {
	if len(toBeAdded) < 1 && len(toBeRemoved) < 1 {
		return nil
	}
	if !r.isLabelsAssignableByUser(ctx, userID, toBeAdded, toBeRemoved) {
		return errors.New("labels are not assignable by user")
	}

	var taskLabels []*tModel.TaskLabels
	for _, labelID := range toBeAdded {
		taskLabels = append(taskLabels, &tModel.TaskLabels{
			TaskID:  taskID,
			LabelID: labelID,
			UserID:  userID,
		})
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if len(toBeRemoved) > 0 {
			if err := r.db.WithContext(ctx).Where("task_id = ? AND user_id = ? AND label_id IN (?)", taskID, userID, toBeRemoved).Delete(&tModel.TaskLabels{}).Error; err != nil {
				return err
			}
		}
		if len(toBeAdded) > 0 {
			if err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "task_id"}, {Name: "label_id"}, {Name: "user_id"}},
				DoNothing: true,
			}).Create(&taskLabels).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *LabelRepository) DeassignLabelFromAllTaskAndDelete(ctx context.Context, userID int, labelID int) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		log := logging.FromContext(ctx)
		var labelCount int64
		if err := tx.Model(&lModel.Label{}).Where("id = ? AND created_by = ?", labelID, userID).Count(&labelCount).Error; err != nil {
			log.Debug(err)
			return err
		}
		if labelCount < 1 {
			return errors.New("label is not owned by user")
		}

		if err := tx.Where("label_id = ?", labelID).Delete(&tModel.TaskLabels{}).Error; err != nil {
			log.Debug("Error deleting task labels")
			return err
		}

		if err := tx.Where("id = ?", labelID).Delete(&lModel.Label{}).Error; err != nil {
			log.Debug("Error deleting label")
			return err
		}

		return nil
	})
}

func (r *LabelRepository) isLabelsOwner(ctx context.Context, userID int, labelIDs []int) bool {
	var count int64
	r.db.WithContext(ctx).Model(&lModel.Label{}).Where("id IN (?) AND user_id = ?", labelIDs, userID).Count(&count)
	return count == 1
}

func (r *LabelRepository) UpdateLabel(ctx context.Context, userID int, label *lModel.Label) error {

	if err := r.db.WithContext(ctx).Model(&lModel.Label{}).Where("id = ? and created_by = ?", label.ID, userID).Updates(label).Error; err != nil {
		return err
	}
	return nil
}
