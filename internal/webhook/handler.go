package webhook

import (
	"fmt"
	"log"
	"regexp"

	"github.com/gitea-jenkins-integ/internal/config"
	"github.com/gitea-jenkins-integ/internal/gitea"
	"github.com/gitea-jenkins-integ/internal/jenkins"
	"github.com/gitea-jenkins-integ/internal/models"
)

type Handler struct {
	config        *config.Config
	giteaClient   *gitea.Client
	jenkinsClient *jenkins.Client
}

func NewHandler(cfg *config.Config) *Handler {
	return &Handler{
		config:        cfg,
		giteaClient:   gitea.NewClient(cfg.Gitea.BaseURL, cfg.Gitea.Token),
		jenkinsClient: jenkins.NewClient(cfg.Jenkins.BaseURL, cfg.Jenkins.Username, cfg.Jenkins.Token),
	}
}

// HandleWebhook обрабатывает вебхук от Gitea
func (h *Handler) HandleWebhook(payload *models.GiteaWebhookPayload) error {
	// Проверяем, что это событие создания PR
	if payload.Action != "opened" {
		log.Printf("Ignoring webhook action: %s", payload.Action)
		return nil
	}

	if payload.PullRequest == nil || payload.Repository == nil {
		return fmt.Errorf("invalid webhook payload: missing pull_request or repository")
	}

	owner := payload.Repository.Owner.Login
	repoName := payload.Repository.Name
	prNumber := payload.PullRequest.Number

	log.Printf("Processing PR #%d in %s/%s", prNumber, owner, repoName)

	// Проверяем, есть ли конфигурация для этого репозитория
	repoConfig := h.config.GetRepositoryConfig(owner, repoName)
	if repoConfig == nil {
		log.Printf("Repository %s/%s is not configured, ignoring", owner, repoName)
		return nil
	}

	// Запускаем асинхронную обработку
	go h.processPRAsync(owner, repoName, prNumber, repoConfig)

	return nil
}

// processPRAsync асинхронно обрабатывает PR
func (h *Handler) processPRAsync(owner, repoName string, prNumber int, repoConfig *config.RepositoryConfig) {
	timeout := h.config.GetTimeout(repoConfig)

	// Формируем паттерн для поиска джобы
	// Заменяем {pr_number} на номер PR, если он есть в паттерне
	jobPattern := h.buildJobPattern(repoConfig.JobPattern, prNumber)

	log.Printf("Waiting for Jenkins job matching pattern '%s' (timeout: %v)", jobPattern, timeout)

	// Ждем создания джобы
	job, err := h.jenkinsClient.WaitForJob(jobPattern, timeout)
	if err != nil {
		log.Printf("Error waiting for job: %v", err)
		comment := fmt.Sprintf("❌ Ошибка при проверке создания джобы в Jenkins: %v", err)
		if err := h.giteaClient.CreateComment(owner, repoName, prNumber, comment); err != nil {
			log.Printf("Failed to create error comment: %v", err)
		}
		return
	}

	if job == nil {
		// Джоба не создалась в течение таймаута
		log.Printf("Jenkins job not found for PR #%d after timeout", prNumber)
		comment := fmt.Sprintf("⏱️ Джоба в Jenkins не была создана в течение %v секунд.\n\nПаттерн поиска: `%s`", int(timeout.Seconds()), jobPattern)
		if err := h.giteaClient.CreateComment(owner, repoName, prNumber, comment); err != nil {
			log.Printf("Failed to create timeout comment: %v", err)
		}
		return
	}

	// Джоба создалась успешно
	log.Printf("Jenkins job found: %s (%s)", job.Name, job.URL)
	comment := fmt.Sprintf("✅ Джоба в Jenkins успешно создана!\n\n**Название:** %s\n**Ссылка:** %s", job.Name, job.URL)
	if err := h.giteaClient.CreateComment(owner, repoName, prNumber, comment); err != nil {
		log.Printf("Failed to create success comment: %v", err)
	}
}

// buildJobPattern заменяет плейсхолдеры в паттерне на реальные значения
func (h *Handler) buildJobPattern(pattern string, prNumber int) string {
	// Заменяем {pr_number} на номер PR
	re := regexp.MustCompile(`\{pr_number\}`)
	return re.ReplaceAllString(pattern, fmt.Sprintf("%d", prNumber))
}
