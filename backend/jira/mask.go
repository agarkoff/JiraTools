package jira

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"

	"jira-tools-web/models"
)

var fakeNames = []string{
	"Иванов Иван", "Петров Пётр", "Сидоров Сидор", "Кузнецов Кузьма",
	"Смирнов Семён", "Васильев Василий", "Попов Павел", "Соколов Степан",
	"Михайлов Михаил", "Новиков Николай", "Фёдоров Фёдор", "Морозов Максим",
	"Волков Виктор", "Алексеев Алексей", "Лебедев Леонид", "Семёнов Сергей",
	"Егоров Евгений", "Павлов Платон", "Козлов Константин", "Степанов Святослав",
}

var fakeSummaries = []string{
	"Реализовать модуль обработки данных",
	"Исправить ошибку валидации формы",
	"Добавить экспорт отчёта в Excel",
	"Оптимизировать запрос к базе данных",
	"Обновить документацию по API",
	"Рефакторинг компонента списка",
	"Настроить деплой через CI/CD",
	"Интеграция со внешним сервисом",
	"Миграция базы данных на новую схему",
	"Покрытие unit-тестами",
	"Добавить кэширование результатов",
	"Логирование операций пользователя",
	"Доработка пользовательского интерфейса",
	"Перевод на новую версию библиотеки",
	"Аудит производительности модуля",
	"Резервное копирование данных",
	"Настройка системы мониторинга",
	"Внедрение схемы безопасности",
	"Поддержка тёмной темы оформления",
	"Локализация интерфейса на английский",
	"Обработка ошибок сетевого слоя",
	"Автотесты для критичных сценариев",
	"Очистка устаревшего кода",
	"Профилирование запросов",
	"Внедрение новой схемы авторизации",
}

func hashIdx(s string, mod int) int {
	h := sha256.Sum256([]byte(s))
	v := binary.BigEndian.Uint32(h[:4])
	return int(v % uint32(mod))
}

func MaskName(s string) string {
	if s == "" {
		return s
	}
	return fakeNames[hashIdx(s, len(fakeNames))]
}

func MaskSummary(s string) string {
	if s == "" {
		return s
	}
	return fakeSummaries[hashIdx(s, len(fakeSummaries))]
}

func maskUser(u *models.User) {
	if u == nil {
		return
	}
	u.DisplayName = MaskName(u.DisplayName)
}

func MaskIssue(issue *models.Issue) {
	issue.Fields.Summary = MaskSummary(issue.Fields.Summary)
	issue.Fields.Description = ""
	maskUser(issue.Fields.Assignee)
	maskUser(issue.Fields.Creator)
	if issue.Fields.Worklog != nil {
		for i := range issue.Fields.Worklog.Worklogs {
			maskUser(issue.Fields.Worklog.Worklogs[i].Author)
		}
	}
}

func MaskWorklogs(wls []models.Worklog) {
	for i := range wls {
		maskUser(wls[i].Author)
	}
}

// MaskRawFields masks summary and assignee/creator inside the raw field map
// returned from DoSearch. Modifies the map in-place.
func MaskRawFields(fields map[string]json.RawMessage) {
	if raw, ok := fields["summary"]; ok && string(raw) != "null" {
		var s string
		if json.Unmarshal(raw, &s) == nil {
			if masked, err := json.Marshal(MaskSummary(s)); err == nil {
				fields["summary"] = masked
			}
		}
	}
	for _, key := range []string{"assignee", "creator"} {
		if raw, ok := fields[key]; ok && string(raw) != "null" {
			var u models.User
			if json.Unmarshal(raw, &u) == nil {
				u.DisplayName = MaskName(u.DisplayName)
				if masked, err := json.Marshal(u); err == nil {
					fields[key] = masked
				}
			}
		}
	}
}
