package label

type Label struct {
	ID        int    `json:"id" gorm:"primary_key"`
	Name      string `json:"name" gorm:"column:name"`
	Color     string `json:"color" gorm:"type:varchar(7);column:color"`
	CreatedBy int    `json:"created_by" gorm:"column:created_by"`
}
