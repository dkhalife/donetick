package task

import (
	"fmt"
	"log"
	"strconv"
	"time"

	tModel "dkhalife.com/tasks/core/internal/models/task"
	lRepo "dkhalife.com/tasks/core/internal/repos/label"
	nRepo "dkhalife.com/tasks/core/internal/repos/notifier"
	tRepo "dkhalife.com/tasks/core/internal/repos/task"
	"dkhalife.com/tasks/core/internal/services/logging"
	notifications "dkhalife.com/tasks/core/internal/services/notifications"
	planner "dkhalife.com/tasks/core/internal/services/planner"
	auth "dkhalife.com/tasks/core/internal/utils/auth"
	jwt "github.com/appleboy/gin-jwt/v2"
	"github.com/gin-gonic/gin"
)

type LabelReq struct {
	LabelID int `json:"id" binding:"required"`
}

type TaskReq struct {
	ID           string              `json:"id"`
	Title        string              `json:"title" binding:"required"`
	NextDueDate  int64               `json:"next_due_date"`
	IsRolling    bool                `json:"is_rolling"`
	Frequency    tModel.Frequency    `json:"frequency"`
	Notification tModel.Notification `json:"notification"`
	Labels       []int               `json:"labels"`
}

type Handler struct {
	tRepo    *tRepo.TaskRepository
	notifier *notifications.Notifier
	nPlanner *planner.NotificationPlanner
	nRepo    *nRepo.NotificationRepository
	lRepo    *lRepo.LabelRepository
}

func NewHandler(cr *tRepo.TaskRepository, nt *notifications.Notifier,
	np *planner.NotificationPlanner, nRepo *nRepo.NotificationRepository, lRepo *lRepo.LabelRepository) *Handler {
	return &Handler{
		tRepo:    cr,
		notifier: nt,
		nPlanner: np,
		nRepo:    nRepo,
		lRepo:    lRepo,
	}
}

func (h *Handler) getTasks(c *gin.Context) {
	u, ok := auth.CurrentUser(c)
	if !ok {
		c.JSON(500, gin.H{
			"error": "Error getting current user",
		})
		return
	}

	tasks, err := h.tRepo.GetTasks(c, u.ID)
	if err != nil {
		c.JSON(500, gin.H{
			"error": "Error getting tasks",
		})
		return
	}

	c.JSON(200, gin.H{
		"tasks": tasks,
	})
}

func (h *Handler) getTask(c *gin.Context) {
	currentUser, ok := auth.CurrentUser(c)
	if !ok {
		c.JSON(500, gin.H{
			"error": "Error getting current user",
		})
		return
	}

	rawID := c.Param("id")
	id, err := strconv.Atoi(rawID)
	if err != nil {
		c.JSON(400, gin.H{
			"error": "Invalid ID",
		})
		return
	}

	task, err := h.tRepo.GetTask(c, id)
	if err != nil {
		c.JSON(500, gin.H{
			"error": "Error getting task",
		})
		return
	}

	if currentUser.ID != task.CreatedBy {
		c.JSON(403, gin.H{
			"error": "You are not allowed to view this task",
		})
		return
	}

	c.JSON(200, gin.H{
		"task": task,
	})
}

func (h *Handler) createTask(c *gin.Context) {
	logger := logging.FromContext(c)
	currentUser, ok := auth.CurrentUser(c)

	logger.Debug("Create task", "currentUser", currentUser)
	if !ok {
		c.JSON(500, gin.H{
			"error": "Error getting current user",
		})
		return
	}

	var TaskReq TaskReq
	if err := c.ShouldBindJSON(&TaskReq); err != nil {
		log.Print(err)
		c.JSON(400, gin.H{
			"error": "Invalid request",
		})
		return
	}

	var dueDate *time.Time

	if TaskReq.NextDueDate != 0 {
		rawDueDate := time.UnixMilli(TaskReq.NextDueDate)
		dueDate = &rawDueDate

	}

	createdTask := &tModel.Task{
		Title:        TaskReq.Title,
		Frequency:    TaskReq.Frequency,
		NextDueDate:  dueDate,
		CreatedBy:    currentUser.ID,
		IsRolling:    TaskReq.IsRolling,
		IsActive:     true,
		Notification: TaskReq.Notification,
		CreatedAt:    time.Now().UTC(),
	}
	id, err := h.tRepo.CreateTask(c, createdTask)
	createdTask.ID = id

	if err != nil {
		c.JSON(500, gin.H{
			"error": "Error creating task",
		})
		return
	}

	if err := h.lRepo.AssignLabelsToTask(c, createdTask.ID, currentUser.ID, TaskReq.Labels); err != nil {
		c.JSON(500, gin.H{
			"error": "Error adding labels",
		})
		return
	}

	go func() {
		h.nPlanner.GenerateNotifications(c, createdTask)
	}()

	c.JSON(200, gin.H{
		"task": id,
	})
}

func (h *Handler) editTask(c *gin.Context) {
	currentUser, ok := auth.CurrentUser(c)
	if !ok {
		c.JSON(500, gin.H{
			"error": "Error getting current user",
		})
		return
	}

	var TaskReq TaskReq
	if err := c.ShouldBindJSON(&TaskReq); err != nil {
		c.JSON(400, gin.H{
			"error": "Invalid request",
		})
		return
	}

	var dueDate *time.Time

	if TaskReq.NextDueDate != 0 {
		rawDueDate := time.UnixMilli(TaskReq.NextDueDate)
		dueDate = &rawDueDate
	}

	taskId, err := strconv.Atoi(TaskReq.ID)
	if err != nil {
		c.JSON(400, gin.H{
			"error": "Invalid task ID",
		})
		return
	}
	oldTask, err := h.tRepo.GetTask(c, taskId)

	if err != nil {
		c.JSON(500, gin.H{
			"error": "Error getting task",
		})
		return
	}

	if currentUser.ID != oldTask.CreatedBy {
		c.JSON(403, gin.H{
			"error": "You are not allowed to edit this task",
		})
		return
	}

	if err := h.lRepo.AssignLabelsToTask(c, taskId, currentUser.ID, TaskReq.Labels); err != nil {
		c.JSON(500, gin.H{
			"error": "Error adding labels",
		})
		return
	}

	updatedTask := &tModel.Task{
		ID:           taskId,
		Title:        TaskReq.Title,
		Frequency:    TaskReq.Frequency,
		NextDueDate:  dueDate,
		CreatedBy:    currentUser.ID,
		IsRolling:    TaskReq.IsRolling,
		Notification: TaskReq.Notification,
		IsActive:     oldTask.IsActive,
		CreatedAt:    oldTask.CreatedAt,
	}
	if err := h.tRepo.UpsertTask(c, updatedTask); err != nil {
		c.JSON(500, gin.H{
			"error": "Error adding task",
		})
		return
	}

	go func() {
		h.nPlanner.GenerateNotifications(c, updatedTask)
	}()

	c.JSON(200, gin.H{})
}

func (h *Handler) deleteTask(c *gin.Context) {
	currentUser, ok := auth.CurrentUser(c)
	if !ok {
		c.JSON(500, gin.H{
			"error": "Error getting current user",
		})
		return
	}

	rawID := c.Param("id")
	id, err := strconv.Atoi(rawID)
	if err != nil {
		c.JSON(400, gin.H{
			"error": "Invalid ID",
		})
		return
	}
	// check if the user is the owner of the task before deleting
	if err := h.tRepo.IsTaskOwner(c, id, currentUser.ID); err != nil {
		c.JSON(403, gin.H{
			"error": "You are not allowed to delete this task",
		})
		return
	}

	if err := h.tRepo.DeleteTask(c, id); err != nil {
		c.JSON(500, gin.H{
			"error": "Error deleting task",
		})
		return
	}
	h.nRepo.DeleteAllTaskNotifications(id)

	c.JSON(200, gin.H{})
}

func (h *Handler) skipTask(c *gin.Context) {
	rawID := c.Param("id")
	id, err := strconv.Atoi(rawID)

	if err != nil {
		c.JSON(400, gin.H{
			"error": "Invalid ID",
		})
		return
	}

	currentUser, ok := auth.CurrentUser(c)
	if !ok {
		c.JSON(500, gin.H{
			"error": "Error getting current user",
		})
		return
	}

	task, err := h.tRepo.GetTask(c, id)
	if err != nil {
		c.JSON(500, gin.H{
			"error": "Error getting task",
		})
		return
	}

	nextDueDate, err := tRepo.ScheduleNextDueDate(task, task.NextDueDate.UTC())
	if err != nil {
		c.JSON(500, gin.H{
			"error": "Error scheduling next due date",
		})
		return
	}

	if err := h.tRepo.CompleteTask(c, task, currentUser.ID, nextDueDate, nil); err != nil {
		c.JSON(500, gin.H{
			"error": "Error completing task",
		})
		return
	}

	updatedTask, err := h.tRepo.GetTask(c, id)
	if err != nil {
		c.JSON(500, gin.H{
			"error": "Error getting task",
		})
		return
	}

	c.JSON(200, gin.H{
		"task": updatedTask,
	})
}

func (h *Handler) updateDueDate(c *gin.Context) {
	currentUser, ok := auth.CurrentUser(c)
	if !ok {
		c.JSON(500, gin.H{
			"error": "Error getting current user",
		})
		return
	}

	type DueDateReq struct {
		DueDate int64 `json:"due_date" binding:"required"`
	}

	var dueDateReq DueDateReq
	if err := c.ShouldBindJSON(&dueDateReq); err != nil {
		log.Print(err)
		c.JSON(400, gin.H{
			"error": "Invalid request",
		})
		return
	}

	rawID := c.Param("id")
	id, err := strconv.Atoi(rawID)
	if err != nil {
		c.JSON(400, gin.H{
			"error": "Invalid ID",
		})
		return
	}

	task, err := h.tRepo.GetTask(c, id)
	if err != nil {
		c.JSON(500, gin.H{
			"error": "Error getting task",
		})
		return
	}

	if currentUser.ID != task.CreatedBy {
		c.JSON(403, gin.H{
			"error": "You are not allowed to update this task",
		})
	}

	dueDate := time.UnixMilli(dueDateReq.DueDate)
	task.NextDueDate = &dueDate
	if err := h.tRepo.UpsertTask(c, task); err != nil {
		c.JSON(500, gin.H{
			"error": "Error updating due date",
		})
		return
	}

	c.JSON(200, gin.H{
		"task": task,
	})
}

func (h *Handler) completeTask(c *gin.Context) {
	currentUser, ok := auth.CurrentUser(c)
	if !ok {
		c.JSON(500, gin.H{
			"error": "Error getting current user",
		})
		return
	}

	completeTaskID := c.Param("id")

	type CompleteReq struct {
		CompletedDate int64 `json:"completed_date"`
	}

	var completeReq CompleteReq
	if err := c.ShouldBindJSON(&completeReq); err != nil {
		log.Print(err)
		c.JSON(400, gin.H{
			"error": "Invalid request",
		})
		return
	}

	var completedDate time.Time
	rawCompletedDate := completeReq.CompletedDate
	if rawCompletedDate == 0 {
		completedDate = time.Now().UTC()
	} else {
		completedDate = time.UnixMilli(int64(completeReq.CompletedDate))
	}

	id, err := strconv.Atoi(completeTaskID)
	if err != nil {
		c.JSON(400, gin.H{
			"error": "Invalid ID",
		})
		return
	}

	task, err := h.tRepo.GetTask(c, id)
	if err != nil {
		c.JSON(500, gin.H{
			"error": "Error getting task",
		})
		return
	}

	var nextDueDate *time.Time
	nextDueDate, err = tRepo.ScheduleNextDueDate(task, completedDate)
	if err != nil {
		c.JSON(500, gin.H{
			"error": fmt.Sprintf("Error scheduling next due date: %s", err),
		})
		return
	}

	if err := h.tRepo.CompleteTask(c, task, currentUser.ID, nextDueDate, &completedDate); err != nil {
		c.JSON(500, gin.H{
			"error": "Error completing task",
		})
		return
	}

	updatedTask, err := h.tRepo.GetTask(c, id)
	if err != nil {
		c.JSON(500, gin.H{
			"error": "Error getting task",
		})
		return
	}

	h.nPlanner.GenerateNotifications(c, updatedTask)

	c.JSON(200, gin.H{
		"task": updatedTask,
	})
}

func (h *Handler) GetTaskHistory(c *gin.Context) {
	rawID := c.Param("id")
	id, err := strconv.Atoi(rawID)
	if err != nil {
		c.JSON(400, gin.H{
			"error": "Invalid ID",
		})
		return
	}

	TaskHistory, err := h.tRepo.GetTaskHistory(c, id)
	if err != nil {
		c.JSON(500, gin.H{
			"error": "Error getting task history",
		})
		return
	}

	c.JSON(200, gin.H{
		"history": TaskHistory,
	})
}

func Routes(router *gin.Engine, h *Handler, auth *jwt.GinJWTMiddleware) {
	tasksRoutes := router.Group("api/v1/tasks")
	tasksRoutes.Use(auth.MiddlewareFunc())
	{
		tasksRoutes.GET("/", h.getTasks)
		tasksRoutes.PUT("/", h.editTask)
		tasksRoutes.POST("/", h.createTask)
		tasksRoutes.GET("/:id", h.getTask)
		tasksRoutes.GET("/:id/history", h.GetTaskHistory)
		tasksRoutes.POST("/:id/do", h.completeTask)
		tasksRoutes.POST("/:id/skip", h.skipTask)
		tasksRoutes.PUT("/:id/dueDate", h.updateDueDate)
		tasksRoutes.DELETE("/:id", h.deleteTask)
	}
}
