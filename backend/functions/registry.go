package functions

import (
	"jira-tools-web/models"
	"jira-tools-web/sse"
)

type Param struct {
	Name     string   `json:"name"`
	Type     string   `json:"type"` // string, number, boolean, select, textarea
	Label    string   `json:"label"`
	Required bool     `json:"required"`
	Default  string   `json:"default,omitempty"`
	Options  []string `json:"options,omitempty"`
}

type FuncDef struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Params      []Param `json:"params"`
	Runner      func(cfg models.JiraConfig, params map[string]string, out *sse.Writer) error `json:"-"`
}

func GetRegistry() []FuncDef {
	return []FuncDef{
		{
			ID:          "orphans",
			Name:        "Задачи-сироты",
			Description: "Найти задачи типа Задача, не привязанные к Историям",
			Params: []Param{
				{Name: "project", Type: "string", Label: "Ключ проекта", Required: true},
			},
			Runner: RunOrphans,
		},
		{
			ID:          "workload",
			Name:        "Загрузка",
			Description: "Показать загрузку пользователей с учётом оценок времени",
			Params: []Param{
				{Name: "project", Type: "string", Label: "Ключи проектов (через запятую)", Required: true},
				{Name: "period", Type: "select", Label: "Период", Default: "all", Options: []string{"all", "week", "month"}},
			},
			Runner: RunWorkload,
		},
		{
			ID:          "estimates",
			Name:        "Анализ оценок",
			Description: "Анализ точности оценок времени по пользователям",
			Params: []Param{
				{Name: "project", Type: "string", Label: "Ключи проектов (через запятую)", Required: true},
				{Name: "worklogs", Type: "boolean", Label: "Использовать ворклоги для точного учёта", Default: "false"},
			},
			Runner: RunEstimates,
		},
		{
			ID:          "epics",
			Name:        "Эпики",
			Description: "Вывести задачи с привязкой к эпикам, при необходимости удалить",
			Params: []Param{
				{Name: "project", Type: "string", Label: "Ключ проекта", Required: true},
				{Name: "epic_field", Type: "string", Label: "Поле Epic Link", Default: "customfield_10109"},
				{Name: "remove_epic", Type: "boolean", Label: "Удалить эпик у задач", Default: "false"},
			},
			Runner: RunEpics,
		},
		{
			ID:          "set-epic",
			Name:        "Установка эпика",
			Description: "Установить эпик для списка задач",
			Params: []Param{
				{Name: "epic_key", Type: "string", Label: "Ключ эпика (например PROJ-123)", Required: true},
				{Name: "task_keys", Type: "textarea", Label: "Ключи задач (по одному на строку)", Required: true},
				{Name: "epic_field", Type: "string", Label: "Поле Epic Link", Default: "customfield_10109"},
			},
			Runner: RunSetEpic,
		},
		{
			ID:          "churn",
			Name:        "Code Churn",
			Description: "Анализ git-истории: задачи с наибольшим количеством изменений кода",
			Params: []Param{
				{Name: "project", Type: "string", Label: "Ключ проекта", Required: true},
				{Name: "repo_path", Type: "string", Label: "Путь к git-репозиторию", Required: true},
				{Name: "limit", Type: "number", Label: "Количество задач в топе", Default: "20"},
			},
			Runner: RunChurn,
		},
		{
			ID:          "check-links",
			Name:        "Проверка связей",
			Description: "Проверить что связи Задача→История имеют тип parentof",
			Params: []Param{
				{Name: "project", Type: "string", Label: "Ключи проектов (через запятую)", Required: true},
				{Name: "fix_parentof", Type: "boolean", Label: "Исправить связи на parentof", Default: "false"},
			},
			Runner: RunCheckLinks,
		},
		{
			ID:          "no-fixversion",
			Name:        "Без fixVersion",
			Description: "Найти задачи с привязанными коммитами/MR из GitLab, но без fixVersion",
			Params: []Param{
				{Name: "project", Type: "string", Label: "Ключи проектов (через запятую)", Required: true},
			},
			Runner: RunNoFixVersion,
		},
	}
}
