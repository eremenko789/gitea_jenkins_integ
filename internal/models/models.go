package models

// GiteaWebhookPayload представляет структуру вебхука от Gitea
type GiteaWebhookPayload struct {
	Action      string       `json:"action"`
	Number      int          `json:"number"`
	PullRequest *PullRequest `json:"pull_request"`
	Repository  *Repository  `json:"repository"`
}

type PullRequest struct {
	ID      int     `json:"id"`
	Number  int     `json:"number"`
	Title   string  `json:"title"`
	State   string  `json:"state"`
	Head    *Branch `json:"head"`
	Base    *Branch `json:"base"`
	HTMLURL string  `json:"html_url"`
}

type Branch struct {
	Ref string `json:"ref"`
	SHA string `json:"sha"`
}

type Repository struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	FullName string `json:"full_name"`
	Owner    *Owner `json:"owner"`
}

type Owner struct {
	Login string `json:"login"`
}

// JenkinsJob представляет информацию о джобе в Jenkins
type JenkinsJob struct {
	Name  string `json:"name"`
	URL   string `json:"url"`
	Color string `json:"color"`
}

// JenkinsJobList представляет список джоб из Jenkins API
type JenkinsJobList struct {
	Jobs []JenkinsJob `json:"jobs"`
}
