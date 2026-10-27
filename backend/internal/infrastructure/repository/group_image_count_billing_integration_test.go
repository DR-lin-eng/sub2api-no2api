//go:build integration

package repository

import "github.com/Wei-Shaw/sub2api/internal/application/service"

func (s *GroupRepoSuite) TestImageCountBillingRoundTrip() {
	group := &service.Group{Name: "per-image", Platform: service.PlatformOpenAI, Status: service.StatusActive,
		RateMultiplier: 1, SubscriptionType: service.SubscriptionTypeStandard, AllowImageGeneration: true, ImageCountBilling: true}
	s.Require().NoError(s.repo.Create(s.ctx, group))
	full, err := s.repo.GetByID(s.ctx, group.ID)
	s.Require().NoError(err)
	s.Require().True(full.ImageCountBilling)
	lite, err := s.repo.GetByIDLite(s.ctx, group.ID)
	s.Require().NoError(err)
	s.Require().True(lite.ImageCountBilling)
	full.ImageCountBilling = false
	s.Require().NoError(s.repo.Update(s.ctx, full))
	lite, err = s.repo.GetByIDLite(s.ctx, group.ID)
	s.Require().NoError(err)
	s.Require().False(lite.ImageCountBilling)
}
