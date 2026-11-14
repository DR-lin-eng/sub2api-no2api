package service

import "context"

type openAIRequestCurrentCheckKey struct{}

// A session's check must run after asynchronous admission, proof and device
// preparation, immediately before the HTTP port can send the request.
func withOpenAIRequestCurrentCheck(ctx context.Context, check func(context.Context) error) context.Context {
	return context.WithValue(ctx, openAIRequestCurrentCheckKey{}, check)
}

func checkOpenAIRequestCurrent(ctx context.Context) error {
	if check, ok := ctx.Value(openAIRequestCurrentCheckKey{}).(func(context.Context) error); ok {
		return check(ctx)
	}
	return nil
}

func (s *chatGPTImageSession) assertCurrent(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	current := s.account
	if s.gateway.accountRepo != nil && !s.account.IsShadow() {
		var err error
		current, err = s.gateway.accountRepo.GetByID(ctx, s.account.ID)
		if err != nil {
			return err
		}
	}
	if current == nil {
		return newOpenAIOAuthPrincipalChangedError()
	}
	owner, err := resolveCredentialAccount(ctx, s.gateway.accountRepo, current)
	if err != nil {
		return err
	}
	if owner == nil || !owner.IsOpenAIOAuth() {
		return newOpenAIOAuthPrincipalChangedError()
	}
	if err := validateOpenAIOAuthRefreshPrincipal(s.credential, owner); err != nil {
		return err
	}
	if err := validateOpenAIOAuthRequestPrincipal(owner, s.token); err != nil {
		return err
	}
	return ctx.Err()
}
