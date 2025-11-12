// Package webhook предоставляет типы для работы с событиями вебхуков от Gitea.
package webhook

import "time"

// PullRequestEvent представляет событие pull request от Gitea.
type PullRequestEvent struct {
	Action      string      `json:"action"`
	Number      int64       `json:"number"`
	PullRequest PullRequest `json:"pull_request"`
	Repository  Repository  `json:"repository"`
	Sender      Sender      `json:"sender"`
	Changes     interface{} `json:"changes,omitempty"`
	Timestamp   time.Time   `json:"-"`
}

// PullRequest представляет информацию о pull request.
type PullRequest struct {
	Number int64  `json:"number"`
	Title  string `json:"title"`
	Body   string `json:"body"`
	URL    string `json:"url"`
}

// Repository представляет информацию о репозитории Gitea.
type Repository struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	FullName string `json:"full_name"`
	HTMLURL  string `json:"html_url"`
}

// Sender представляет информацию об отправителе события.
type Sender struct {
	ID       int64  `json:"id"`
	Login    string `json:"login"`
	FullName string `json:"full_name"`
}

// DisplayName возвращает отображаемое имя pull request.
// Если заголовок не пуст, возвращает заголовок, иначе возвращает "PR".
func (p PullRequest) DisplayName() string {
	if p.Title != "" {
		return p.Title
	}
	return "PR"
}
