package service

import (
	"arenea/backend/internal/domain"
	"arenea/backend/internal/repository"
)

type TeamService struct {
	repo repository.Store
}

func NewTeamService(repo repository.Store) *TeamService {
	return &TeamService{repo: repo}
}

func (s *TeamService) List() ([]domain.Team, error) {
	return s.repo.ListTeams()
}
