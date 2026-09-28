package monitoring

import "github.com/Beyondtech-ID/ms-monitoring-qa-be/internal/repository"

type Project = repository.Project
type Member = repository.Member
type Allocation = repository.Allocation
type JiraIssue = repository.JiraIssue
type QaseCase = repository.QaseCase
type QaseRun = repository.QaseRun
type QaseRunCase = repository.QaseRunCase
type QaseResult = repository.QaseResult
type QaseDefect = repository.QaseDefect
type SyncJob = repository.SyncJob
type SyncStep = repository.SyncStep
type SyncEvent = repository.SyncEvent
type SyncCursor = repository.SyncCursor

func Models() []any { return repository.Models() }
