package main

import (
	"bytes"
	"fmt"
	"html/template"
	"sort"
	"strings"
	"time"

	"github.com/axcherednikov/codex-insights/internal/analyze"
	"github.com/axcherednikov/codex-insights/internal/i18n"
)

// HTMLReportCounts contains report-level counts, not session records.
type HTMLReportCounts struct {
	UserSessions  int
	Tasks         int
	FollowupPairs int
}

// HTMLLabelCount is a deterministic aggregate of one semantic label. Label
// counts are intentionally accepted instead of per-turn results or IDs.
type HTMLLabelCount struct {
	Label string
	Count int
}

// HTMLReportLabels contains only pre-aggregated semantic labels.
type HTMLReportLabels struct {
	Steering        []HTMLLabelCount
	Reasons         []HTMLLabelCount
	PromptQuality   []HTMLLabelCount
	AgentsRules     []HTMLLabelCount
	SkillCandidates []HTMLLabelCount
	Validation      []HTMLLabelCount
}

// HTMLReportInput is deliberately aggregate-only. It accepts semantic labels
// and numeric analysis results, but has no path for prompts, answers, turns,
// interactions, or follow-up text.
type HTMLReportInput struct {
	Translator  i18n.Translator
	WindowLabel string
	WindowStart time.Time
	WindowEnd   time.Time
	Counts      HTMLReportCounts
	Semantic    analyze.SemanticStats

	Effectiveness analyze.EffectivenessAnalysis
	Labels        HTMLReportLabels
}

type htmlReportView struct {
	Title                string
	Window               string
	Overview             string
	Strengths            string
	GrowthAreas          string
	Recommendations      string
	SupportingMetrics    string
	Methodology          string
	Privacy              string
	ObservedLabel        string
	WhyLabel             string
	ActionLabel          string
	ExampleLabel         string
	BeforeLabel          string
	AfterLabel           string
	ChecklistLabel       string
	Insufficient         bool
	InsufficientTitle    string
	InsufficientBody     string
	InsufficientNext     string
	Metrics              []htmlMetric
	StrengthItems        []string
	GrowthItems          []string
	RecommendationsList  []htmlRecommendation
	RecommendationsEmpty string
	MethodologyItems     []string
}

type htmlMetric struct {
	Label string
	Value string
	Rate  float64
}

type htmlRecommendation struct {
	Priority    string
	Title       string
	Observed    string
	Why         string
	Action      string
	BeforeLabel string
	Before      string
	AfterLabel  string
	After       string
	Checklist   []string
}

type htmlCopy struct {
	title, overview, strengths, growthAreas, recommendations, supportingMetrics, methodology, privacy                           string
	insufficientTitle, insufficientBody, insufficientNext                                                                       string
	recommendationsEmpty                                                                                                        string
	observed, why, action, exampleLabel, before, after, checklist                                                               string
	high, medium, low                                                                                                           string
	completedTasks, tasks, steeringRate, followups, sessions, averageTokens, averageSeconds, averageTools, evaluated, cacheHits string
	methodologyOne, methodologyTwo, methodologyThree                                                                            string
	strengthNoData, growthNoData, strengthLowest, growthHighest                                                                 string
	validationTitle, validationObserved, validationWhy, validationAction, validationBefore, validationAfter                     string
	reasonTitle, reasonObserved, reasonWhy, reasonAction, reasonBefore, reasonAfter                                             string
	promptTitle, promptObserved, promptWhy, promptAction, promptBefore, promptAfter                                             string
	ruleTitle, ruleObserved, ruleWhy, ruleAction, ruleBefore, ruleAfter                                                         string
	skillTitle, skillObserved, skillWhy, skillAction, skillBefore, skillAfter                                                   string
	generalTitle, generalObserved, generalWhy, generalAction, generalBefore, generalAfter                                       string
	checkOne, checkTwo, checkThree                                                                                              string
}

func englishHTMLCopy() htmlCopy {
	return htmlCopy{
		title: "Codex Insights — analysis report", overview: "Overview", strengths: "Strengths", growthAreas: "Growth areas", recommendations: "Recommendations", supportingMetrics: "Supporting metrics", methodology: "Methodology & privacy", privacy: "This report uses aggregate counts and semantic labels only. It does not include prompts, answers, or session text.",
		insufficientTitle: "Not enough evidence yet", insufficientBody: "No reliable conclusions can be drawn from the available aggregate observations.", insufficientNext: "Collect more completed tasks with observable follow-up outcomes, then run the report again.", recommendationsEmpty: "No actionable recommendation is supported by the current aggregate evidence.",
		observed: "What was observed", why: "Why it matters", action: "What to do", exampleLabel: "Synthetic example", before: "Before", after: "After", checklist: "Practical checklist", high: "High priority", medium: "Medium priority", low: "Low priority",
		completedTasks: "completed tasks with outcomes", tasks: "tasks", steeringRate: "steering rate", followups: "follow-up pairs", sessions: "sessions", averageTokens: "average tokens", averageSeconds: "average seconds", averageTools: "average tool calls", evaluated: "semantic records evaluated", cacheHits: "semantic cache hits",
		methodologyOne: "Aggregates are computed from completed tasks with available semantic labels; small cohorts are not treated as reliable comparisons.", methodologyTwo: "Recommendations are curated deterministic templates selected from aggregate label frequencies. No additional Judge call is made.", methodologyThree: "Examples are synthetic and are not copied from your sessions.",
		strengthNoData: "A reliable strength pattern is not available yet.", growthNoData: "A reliable growth pattern is not available yet.", strengthLowest: "%s currently has the lowest observed steering rate among meaningful task cohorts (%s).", growthHighest: "%s currently has the highest observed steering rate among meaningful task cohorts (%s).",
		validationTitle: "Make the final validation step explicit", validationObserved: "A validation signal appears in the labeled steering cases: %s.", validationWhy: "A visible final check catches mismatches before they become another correction cycle.", validationAction: "Name the exact check that proves the requested result and run it before reporting completion.", validationBefore: "Done — changes applied.", validationAfter: "Done — changes applied; go test ./... passes and the output was inspected.",
		reasonTitle: "Address the most common correction cause", reasonObserved: "The most common correction reason is %s.", reasonWhy: "Naming a recurring failure mode makes prevention concrete instead of relying on memory.", reasonAction: "Add one targeted guardrail for this failure mode to the task plan or review checklist.", reasonBefore: "The request was interpreted and implemented.", reasonAfter: "The request was interpreted; the risk was named and checked before implementation.",
		promptTitle: "Make acceptance criteria concrete", promptObserved: "Prompt-quality labels most often point to %s.", promptWhy: "A shared definition of done gives the implementation a stable target and makes review faster.", promptAction: "Add constraints, scope boundaries, and one or two observable acceptance checks to the task request.", promptBefore: "Please improve the report.", promptAfter: "Improve the report; keep the API unchanged, add an accessible empty state, and verify it with focused tests.",
		ruleTitle: "Turn the recurring lesson into a project rule", ruleObserved: "The rule signal in the labeled cases is: %s.", ruleWhy: "A short durable rule can prevent the same class of correction across future tasks.", ruleAction: "Capture this guidance in the project instructions and review it when starting a related task.", ruleBefore: "Use your best judgment.", ruleAfter: "Inspect existing code first, preserve local conventions, and run the relevant checks before completion.",
		skillTitle: "Package the repeated workflow", skillObserved: "A reusable workflow candidate appears in the labels: %s.", skillWhy: "A repeatable workflow reduces setup overhead and makes quality checks easier to remember.", skillAction: "Prototype the workflow as a small reusable Skill or checklist, then test it on a new task.", skillBefore: "Handle this carefully.", skillAfter: "Follow the verification workflow: inspect, implement the smallest change, run focused checks, and inspect the result.",
		generalTitle: "Add a lightweight completion check", generalObserved: "The aggregate behavior results include correction cycles (%s steering cases).", generalWhy: "A short final review is a low-cost way to catch missing requirements before handoff.", generalAction: "Before finishing, compare the result with the requested scope and run the narrowest meaningful check.", generalBefore: "Implemented.", generalAfter: "Implemented; scope checked, focused test run, and output reviewed.",
		checkOne: "Restate the requested outcome in one sentence.", checkTwo: "Run the narrowest meaningful validation check.", checkThree: "Inspect the result for scope, format, and accessibility.",
	}
}

func russianHTMLCopy() htmlCopy {
	return htmlCopy{
		title: "Codex Insights — аналитический отчёт", overview: "Обзор", strengths: "Сильные стороны", growthAreas: "Зоны роста", recommendations: "Рекомендации", supportingMetrics: "Дополнительные метрики", methodology: "Методология и приватность", privacy: "Отчёт использует только агрегированные числа и семантические метки. Промпты, ответы и текст сессий не включаются.",
		insufficientTitle: "Пока недостаточно данных", insufficientBody: "По доступным агрегированным наблюдениям нельзя сделать надёжные выводы.", insufficientNext: "Соберите больше завершённых задач с наблюдаемыми результатами продолжения и запустите отчёт снова.", recommendationsEmpty: "Нет рекомендаций, подтверждённых текущими агрегированными данными.",
		observed: "Что наблюдалось", why: "Почему это важно", action: "Что делать", exampleLabel: "Синтетический пример", before: "До", after: "После", checklist: "Практический чек-лист", high: "Высокий приоритет", medium: "Средний приоритет", low: "Низкий приоритет",
		completedTasks: "завершённых задач с результатом", tasks: "задач", steeringRate: "доля корректировок", followups: "пар продолжения", sessions: "сессий", averageTokens: "средние токены", averageSeconds: "средние секунды", averageTools: "средние вызовы инструментов", evaluated: "семантических оценок", cacheHits: "попаданий в семантический кеш",
		methodologyOne: "Агрегаты строятся по завершённым задачам с доступными семантическими метками; маленькие когорты не считаются надёжным сравнением.", methodologyTwo: "Рекомендации — это детерминированные шаблоны, выбранные по частотам агрегированных меток. Дополнительный вызов оценщика не выполняется.", methodologyThree: "Примеры синтетические и не скопированы из ваших сессий.",
		strengthNoData: "Надёжная картина сильных сторон пока недоступна.", growthNoData: "Надёжная картина зон роста пока недоступна.", strengthLowest: "У типа «%s» сейчас самая низкая наблюдаемая доля корректировок среди значимых когорт (%s).", growthHighest: "У типа «%s» сейчас самая высокая наблюдаемая доля корректировок среди значимых когорт (%s).",
		validationTitle: "Сделайте финальную проверку явной", validationObserved: "Сигнал о проверке встречается среди размеченных случаев корректировки: %s.", validationWhy: "Понятная финальная проверка помогает поймать несоответствие до нового цикла корректировки.", validationAction: "Назовите точную проверку, подтверждающую результат, и выполните её до сообщения о готовности.", validationBefore: "Готово — изменения внесены.", validationAfter: "Готово — изменения внесены; go test ./... пройден, результат проверен.",
		reasonTitle: "Разберите самую частую причину корректировок", reasonObserved: "Самая частая причина корректировок — %s.", reasonWhy: "Названный повторяющийся сбой легче предотвращать, чем пытаться держать в памяти.", reasonAction: "Добавьте один точечный барьер для этой причины в план задачи или чек-лист ревью.", reasonBefore: "Задача понята и реализована.", reasonAfter: "Задача понята; риск назван и проверен до реализации.",
		promptTitle: "Сделайте критерии готовности конкретными", promptObserved: "Метки качества постановки чаще всего указывают на проблему: %s.", promptWhy: "Общее определение готовности задаёт устойчивую цель и ускоряет ревью.", promptAction: "Добавляйте ограничения, границы изменений и одну-две наблюдаемые проверки готовности.", promptBefore: "Улучшите отчёт.", promptAfter: "Улучшите отчёт; не меняйте API, добавьте доступное пустое состояние и проверьте его сфокусированными тестами.",
		ruleTitle: "Превратите повторяющийся вывод в правило проекта", ruleObserved: "Сигнал правила в размеченных случаях: %s.", ruleWhy: "Короткое устойчивое правило помогает предотвращать такой класс корректировок в будущих задачах.", ruleAction: "Зафиксируйте рекомендацию в инструкциях проекта и сверяйтесь с ней в похожих задачах.", ruleBefore: "Действуйте по ситуации.", ruleAfter: "Сначала изучите существующий код, сохраните местные соглашения и перед завершением запустите нужные проверки.",
		skillTitle: "Упакуйте повторяющийся процесс", skillObserved: "Кандидат на переиспользуемый процесс встречается среди меток: %s.", skillWhy: "Повторяемый процесс снижает подготовительную работу и помогает не забывать проверки качества.", skillAction: "Сделайте небольшой переиспользуемый Skill или чек-лист и проверьте его на новой задаче.", skillBefore: "Сделайте это внимательно.", skillAfter: "Следуйте процессу проверки: изучить, внести минимальное изменение, запустить проверки и осмотреть результат.",
		generalTitle: "Добавьте лёгкую проверку перед завершением", generalObserved: "В агрегированных результатах есть циклы корректировок (%s случаев).", generalWhy: "Короткий финальный просмотр помогает недорого заметить пропущенные требования до передачи результата.", generalAction: "Перед завершением сверяйте результат с границами задачи и запускайте самую узкую полезную проверку.", generalBefore: "Реализовано.", generalAfter: "Реализовано; границы проверены, тест запущен, результат просмотрен.",
		checkOne: "Сформулируйте ожидаемый результат задачи одним предложением.", checkTwo: "Запустите самую узкую полезную проверку.", checkThree: "Проверьте результат на границы, формат и доступность.",
	}
}

// RenderHTMLReport renders one deterministic, standalone HTML document.
// It does not invoke a Judge and never receives session conversation text.
func RenderHTMLReport(input HTMLReportInput) (string, error) {
	copy := englishHTMLCopy()
	if input.Translator.Language == i18n.Russian {
		copy = russianHTMLCopy()
	}
	view := buildHTMLReportView(input, copy)
	var output bytes.Buffer
	if err := htmlReportTemplate.Execute(&output, view); err != nil {
		return "", err
	}
	return output.String(), nil
}

func buildHTMLReportView(input HTMLReportInput, copy htmlCopy) htmlReportView {
	window := input.WindowLabel
	if window == "" {
		window = formatHTMLWindow(input.WindowStart, input.WindowEnd)
	}
	if window == "" {
		window = "all history"
		if input.Translator.Language == i18n.Russian {
			window = "вся история"
		}
	}

	steering := countHTMLSteering(input.Labels.Steering)
	steeringSamples := countHTMLBehaviorLabels(input.Labels.Steering)
	if input.Effectiveness.Overall.Samples > steeringSamples {
		steeringSamples = input.Effectiveness.Overall.Samples
	}
	steeringRateValue := 0.0
	if steeringSamples > 0 {
		steeringRateValue = 100 * float64(steering) / float64(steeringSamples)
	}
	if input.Effectiveness.Overall.Samples > 0 {
		steeringRateValue = 100 * steeringRate(input.Effectiveness.Overall)
	}
	metrics := []htmlMetric{
		{Label: copy.completedTasks, Value: formatInt(input.Effectiveness.Overall.Samples), Rate: clampHTMLRate(float64(input.Effectiveness.Overall.Samples), 100)},
		{Label: copy.tasks, Value: formatInt(input.Counts.Tasks), Rate: clampHTMLRate(float64(input.Counts.Tasks), float64(maxInt(input.Counts.Tasks, 1)))},
		{Label: copy.steeringRate, Value: formatPercent(steeringRateValue), Rate: clampHTMLRate(steeringRateValue, 100)},
		{Label: copy.followups, Value: formatInt(input.Counts.FollowupPairs), Rate: clampHTMLRate(float64(input.Counts.FollowupPairs), float64(maxInt(input.Counts.Tasks, 1)))},
		{Label: copy.sessions, Value: formatInt(input.Counts.UserSessions), Rate: clampHTMLRate(float64(input.Counts.UserSessions), float64(maxInt(input.Counts.UserSessions, 1)))},
	}
	if input.Effectiveness.Overall.Samples > 0 {
		metrics = append(metrics,
			htmlMetric{Label: copy.averageTokens, Value: formatFloat(input.Effectiveness.Overall.AverageTokens), Rate: 0},
			htmlMetric{Label: copy.averageSeconds, Value: formatFloat(input.Effectiveness.Overall.AverageSeconds), Rate: 0},
			htmlMetric{Label: copy.averageTools, Value: formatFloat(input.Effectiveness.Overall.AverageToolCalls), Rate: 0},
		)
	}
	if input.Semantic.EvaluatedRecords > 0 || input.Semantic.CacheHits > 0 {
		metrics = append(metrics,
			htmlMetric{Label: copy.evaluated, Value: formatInt(input.Semantic.EvaluatedRecords), Rate: 0},
			htmlMetric{Label: copy.cacheHits, Value: formatInt(input.Semantic.CacheHits), Rate: 0},
		)
	}

	strengths := make([]string, 0, 2)
	growth := make([]string, 0, 2)
	if strongest, weakest, comparable := htmlCohortExtremes(input.Effectiveness.TaskTypeStats); input.Effectiveness.Overall.Samples >= analyze.MinimumCohortSize && comparable {
		strengths = append(strengths, sprintfHTML(copy.strengthLowest, input.Translator.TaskType(strongest.TaskType), formatPercent(100*steeringRate(strongest.Stats))))
		growth = append(growth, sprintfHTML(copy.growthHighest, input.Translator.TaskType(weakest.TaskType), formatPercent(100*steeringRate(weakest.Stats))))
	} else {
		strengths = append(strengths, copy.strengthNoData)
		growth = append(growth, copy.growthNoData)
	}

	insufficient := input.Effectiveness.Overall.Samples < analyze.MinimumCohortSize
	recommendations := buildHTMLRecommendations(input, copy, !insufficient)
	return htmlReportView{
		Title: copy.title, Window: window, Overview: copy.overview, Strengths: copy.strengths, GrowthAreas: copy.growthAreas, Recommendations: copy.recommendations, SupportingMetrics: copy.supportingMetrics, Methodology: copy.methodology, Privacy: copy.privacy,
		ObservedLabel: copy.observed, WhyLabel: copy.why, ActionLabel: copy.action, ExampleLabel: copy.exampleLabel, BeforeLabel: copy.before, AfterLabel: copy.after, ChecklistLabel: copy.checklist,
		Insufficient: insufficient, InsufficientTitle: copy.insufficientTitle, InsufficientBody: copy.insufficientBody, InsufficientNext: copy.insufficientNext,
		Metrics: metrics, StrengthItems: strengths, GrowthItems: growth, RecommendationsList: recommendations, RecommendationsEmpty: copy.recommendationsEmpty,
		MethodologyItems: []string{copy.methodologyOne, copy.methodologyTwo, copy.methodologyThree},
	}
}

func buildHTMLRecommendations(input HTMLReportInput, copy htmlCopy, sufficientEvidence bool) []htmlRecommendation {
	result := make([]htmlRecommendation, 0, 4)
	if !sufficientEvidence {
		return result
	}
	if validation := topHTMLLabel(input.Labels.Validation, true); validation != nil {
		result = append(result, htmlRecommendation{Priority: copy.high, Title: copy.validationTitle, Observed: sprintfHTML(copy.validationObserved, labelCountText(input.Translator.ValidationType(validation.Label), validation.Count)), Why: copy.validationWhy, Action: copy.validationAction, BeforeLabel: copy.before, Before: copy.validationBefore, AfterLabel: copy.after, After: copy.validationAfter, Checklist: htmlChecklist(copy)})
	}
	if reason := topHTMLLabel(input.Labels.Reasons, false); reason != nil {
		result = append(result, htmlRecommendation{Priority: copy.high, Title: copy.reasonTitle, Observed: sprintfHTML(copy.reasonObserved, labelCountText(input.Translator.SteeringReason(reason.Label), reason.Count)), Why: copy.reasonWhy, Action: copy.reasonAction, BeforeLabel: copy.before, Before: copy.reasonBefore, AfterLabel: copy.after, After: copy.reasonAfter, Checklist: htmlChecklist(copy)})
	}
	if issue := topHTMLLabel(input.Labels.PromptQuality, true); issue != nil {
		result = append(result, htmlRecommendation{Priority: copy.medium, Title: copy.promptTitle, Observed: sprintfHTML(copy.promptObserved, labelCountText(input.Translator.PromptQualityIssue(issue.Label), issue.Count)), Why: copy.promptWhy, Action: copy.promptAction, BeforeLabel: copy.before, Before: copy.promptBefore, AfterLabel: copy.after, After: copy.promptAfter, Checklist: htmlChecklist(copy)})
	}
	if rule := topHTMLLabel(input.Labels.AgentsRules, true); rule != nil {
		result = append(result, htmlRecommendation{Priority: copy.medium, Title: copy.ruleTitle, Observed: sprintfHTML(copy.ruleObserved, labelCountText(input.Translator.AgentsRule(rule.Label), rule.Count)), Why: copy.ruleWhy, Action: copy.ruleAction, BeforeLabel: copy.before, Before: copy.ruleBefore, AfterLabel: copy.after, After: copy.ruleAfter, Checklist: htmlChecklist(copy)})
	}
	if candidates := topHTMLLabel(input.Labels.SkillCandidates, true); candidates != nil && candidates.Count >= 2 {
		result = append(result, htmlRecommendation{Priority: copy.low, Title: copy.skillTitle, Observed: sprintfHTML(copy.skillObserved, labelCountText(input.Translator.SkillCandidate(candidates.Label), candidates.Count)), Why: copy.skillWhy, Action: copy.skillAction, BeforeLabel: copy.before, Before: copy.skillBefore, AfterLabel: copy.after, After: copy.skillAfter, Checklist: htmlChecklist(copy)})
	}
	steering := countHTMLSteering(input.Labels.Steering)
	if len(result) == 0 && steering > 0 {
		result = append(result, htmlRecommendation{Priority: copy.low, Title: copy.generalTitle, Observed: sprintfHTML(copy.generalObserved, steering), Why: copy.generalWhy, Action: copy.generalAction, BeforeLabel: copy.before, Before: copy.generalBefore, AfterLabel: copy.after, After: copy.generalAfter, Checklist: htmlChecklist(copy)})
	}
	return result
}

func htmlChecklist(copy htmlCopy) []string {
	return []string{copy.checkOne, copy.checkTwo, copy.checkThree}
}

func countHTMLSteering(results []HTMLLabelCount) int {
	count := 0
	for _, result := range results {
		if result.Label == "steering" && result.Count > 0 {
			count += result.Count
		}
	}
	return count
}

func countHTMLBehaviorLabels(results []HTMLLabelCount) int {
	count := 0
	for _, result := range results {
		if result.Count > 0 {
			count += result.Count
		}
	}
	return count
}

func labelCountText(label string, count int) string {
	return fmt.Sprintf("%s (%d)", label, count)
}

func topHTMLLabel(labels []HTMLLabelCount, excludeOther bool) *HTMLLabelCount {
	candidates := make([]HTMLLabelCount, 0, len(labels))
	for _, label := range labels {
		if label.Label == "" || label.Count <= 0 || (excludeOther && label.Label == "other") {
			continue
		}
		candidates = append(candidates, label)
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Count != candidates[j].Count {
			return candidates[i].Count > candidates[j].Count
		}
		return candidates[i].Label < candidates[j].Label
	})
	if len(candidates) == 0 {
		return nil
	}
	return &candidates[0]
}

func htmlCohortExtremes(stats []analyze.TaskTypeEffectiveness) (low, high *analyze.TaskTypeEffectiveness, comparable bool) {
	meaningful := make([]analyze.TaskTypeEffectiveness, 0, len(stats))
	for _, stat := range stats {
		if isActionableTaskType(stat.TaskType) && stat.Stats.Samples >= analyze.MinimumCohortSize {
			meaningful = append(meaningful, stat)
		}
	}
	if len(meaningful) < 2 {
		return nil, nil, false
	}
	sort.Slice(meaningful, func(i, j int) bool {
		left, right := steeringRate(meaningful[i].Stats), steeringRate(meaningful[j].Stats)
		if left != right {
			return left < right
		}
		return meaningful[i].TaskType < meaningful[j].TaskType
	})
	if steeringRate(meaningful[0].Stats) == steeringRate(meaningful[len(meaningful)-1].Stats) {
		return nil, nil, false
	}
	return &meaningful[0], &meaningful[len(meaningful)-1], true
}

func formatHTMLWindow(start, end time.Time) string {
	if start.IsZero() && end.IsZero() {
		return ""
	}
	if start.IsZero() {
		return end.UTC().Format("2006-01-02")
	}
	if end.IsZero() {
		return start.UTC().Format("2006-01-02")
	}
	return start.UTC().Format("2006-01-02") + " – " + end.UTC().Format("2006-01-02")
}

func formatInt(value int) string         { return strings.TrimSpace(sprintfHTML("%d", value)) }
func formatFloat(value float64) string   { return sprintfHTML("%.1f", value) }
func formatPercent(value float64) string { return sprintfHTML("%.1f%%", value) }
func sprintfHTML(format string, values ...any) string {
	return strings.TrimSpace(sprintf(format, values...))
}

func sprintf(format string, values ...any) string {
	// Kept in one helper so all user-facing interpolations pass through the
	// html/template boundary instead of being concatenated into markup.
	return fmt.Sprintf(format, values...)
}

func clampHTMLRate(value, maximum float64) float64 {
	if maximum <= 0 || value <= 0 {
		return 0
	}
	if value >= maximum {
		return 100
	}
	return 100 * value / maximum
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}

var htmlReportTemplate = template.Must(template.New("html-report").Parse(`<!doctype html>
<html lang="{{if eq .Title "Codex Insights — аналитический отчёт"}}ru{{else}}en{{end}}">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}}</title>
<style>
:root { color-scheme: light; --ink:#172033; --muted:#5f6b7a; --line:#dfe5ec; --surface:#f6f8fb; --accent:#2f6feb; --accent-soft:#eaf1ff; --warning:#8a5b00; --warning-soft:#fff8e6; }
* { box-sizing:border-box; }
body { margin:0; background:var(--surface); color:var(--ink); font:16px/1.55 system-ui,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif; }
main { max-width:1080px; margin:0 auto; padding:32px 20px 56px; }
header { background:linear-gradient(135deg,#172033,#2f6feb); color:#fff; border-radius:22px; padding:32px; margin-bottom:20px; box-shadow:0 12px 32px rgba(23,32,51,.16); }
h1,h2,h3 { line-height:1.2; margin:0 0 12px; }
h1 { font-size:clamp(1.8rem,4vw,2.7rem); }
h2 { font-size:1.45rem; }
h3 { font-size:1.05rem; }
p { margin:0 0 12px; }
.window { color:#d7e4ff; }
section { background:#fff; border:1px solid var(--line); border-radius:18px; padding:24px; margin:20px 0; }
.metrics { display:grid; grid-template-columns:repeat(auto-fit,minmax(170px,1fr)); gap:12px; margin-top:20px; }
.metric { background:var(--surface); border:1px solid var(--line); border-radius:14px; padding:16px; }
.metric strong { display:block; font-size:1.45rem; margin-bottom:3px; }
.metric span { color:var(--muted); font-size:.9rem; }
.bar { height:6px; background:#dce7fb; border-radius:99px; overflow:hidden; margin-top:12px; }
.bar i { display:block; height:100%; background:var(--accent); border-radius:inherit; }
.two-col { display:grid; grid-template-columns:repeat(auto-fit,minmax(280px,1fr)); gap:20px; }
ul { margin:10px 0 0; padding-left:22px; }
li + li { margin-top:7px; }
.notice { background:var(--warning-soft); border-color:#efd48a; color:var(--warning); }
.recommendation { border:1px solid var(--line); border-radius:15px; padding:20px; margin-top:16px; }
.recommendation:first-child { margin-top:0; }
.priority { display:inline-block; color:var(--accent); background:var(--accent-soft); border-radius:99px; padding:3px 10px; font-size:.78rem; font-weight:700; margin-bottom:10px; }
.label { color:var(--muted); font-weight:700; font-size:.86rem; text-transform:uppercase; letter-spacing:.04em; }
.example { background:var(--surface); border-radius:12px; padding:14px; margin:14px 0; }
.example p { margin:5px 0 0; }
footer { color:var(--muted); font-size:.9rem; padding:4px 4px 0; }
@media (max-width:600px) { main { padding:18px 12px 36px; } header, section { padding:20px; border-radius:16px; } }
@media (prefers-reduced-motion:reduce) { * { scroll-behavior:auto !important; } }
</style>
</head>
<body>
<main>
<header><h1>{{.Title}}</h1><p class="window">{{.Window}}</p></header>
<section aria-labelledby="overview"><h2 id="overview">{{.Overview}}</h2>
{{if .Insufficient}}<div class="notice" role="status"><h3>{{.InsufficientTitle}}</h3><p>{{.InsufficientBody}}</p><p>{{.InsufficientNext}}</p></div>{{end}}
<div class="metrics" aria-label="{{.SupportingMetrics}}">{{range .Metrics}}<div class="metric"><strong>{{.Value}}</strong><span>{{.Label}}</span>{{if .Rate}}<div class="bar" aria-hidden="true"><i style="width: {{printf "%.1f" .Rate}}%"></i></div>{{end}}</div>{{end}}</div></section>
<div class="two-col"><section aria-labelledby="strengths"><h2 id="strengths">{{.Strengths}}</h2><ul>{{range .StrengthItems}}<li>{{.}}</li>{{end}}</ul></section>
<section aria-labelledby="growth"><h2 id="growth">{{.GrowthAreas}}</h2><ul>{{range .GrowthItems}}<li>{{.}}</li>{{end}}</ul></section></div>
<section aria-labelledby="recommendations"><h2 id="recommendations">{{.Recommendations}}</h2>{{if .RecommendationsList}}{{range .RecommendationsList}}<article class="recommendation"><span class="priority">{{.Priority}}</span><h3>{{.Title}}</h3><p><span class="label">{{$.ObservedLabel}}</span> — {{.Observed}}</p><p><span class="label">{{$.WhyLabel}}</span> — {{.Why}}</p><p><span class="label">{{$.ActionLabel}}</span> — {{.Action}}</p><div class="example"><strong>{{$.ExampleLabel}}</strong><p><b>{{.BeforeLabel}}:</b> {{.Before}}</p><p><b>{{.AfterLabel}}:</b> {{.After}}</p></div><p class="label">{{$.ChecklistLabel}}</p><ul>{{range .Checklist}}<li>{{.}}</li>{{end}}</ul></article>{{end}}{{else}}<p>{{.RecommendationsEmpty}}</p>{{end}}</section>
<section aria-labelledby="methodology"><h2 id="methodology">{{.Methodology}}</h2><ul>{{range .MethodologyItems}}<li>{{.}}</li>{{end}}</ul><footer>{{.Privacy}}</footer></section>
</main>
</body>
</html>`))
