package service

import (
	"arenea/backend/internal/domain"
	"arenea/backend/internal/repository"
)

type SessionService struct {
	repo repository.Store
}

func NewSessionService(repo repository.Store) *SessionService {
	return &SessionService{repo: repo}
}

func (s *SessionService) Create(in domain.Session) (domain.Session, error) {
	in.ID = newID()
	return s.repo.CreateSession(in)
}

func (s *SessionService) List(agentID string) ([]domain.Session, error) {
	return s.repo.ListSessions(agentID)
}

func (s *SessionService) ListTeam(teamID string) ([]domain.Session, error) {
	return s.repo.ListTeamSessions(teamID)
}

func (s *SessionService) Delete(id string) error {
	return s.repo.DeleteSession(id)
}

func (s *SessionService) DeleteByAgent(agentID string) error {
	return s.repo.DeleteSessionsByAgentID(agentID)
}
