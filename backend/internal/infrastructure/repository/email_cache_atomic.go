package repository

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/redis/go-redis/v9"
)

// One key and one round trip: the old JSON representation and its TTL remain
// readable by existing nodes. A successful check also consumes the code.
var verifyEmailCodeScript = redis.NewScript(`
local raw = redis.call('GET', KEYS[1])
if not raw or redis.call('PTTL', KEYS[1]) <= 0 then return -1 end
local ok, data = pcall(cjson.decode, raw)
if not ok or type(data) ~= 'table' or type(data.Code) ~= 'string' or data.Code == '' then return -1 end
local attempts = tonumber(data.Attempts) or 0
local max = tonumber(ARGV[2])
if not max or max <= 0 then return -1 end
if attempts >= max then return -2 end
local match = #data.Code == #ARGV[1]
for i = 1, #data.Code do
  if string.byte(data.Code, i) ~= string.byte(ARGV[1], i) then match = false end
end
if match then
  redis.call('DEL', KEYS[1])
  return 1
end
data.Attempts = attempts + 1
redis.call('SET', KEYS[1], cjson.encode(data), 'KEEPTTL')
if data.Attempts >= max then return -2 end
return -1
`)

var consumeEmailResetTokenScript = redis.NewScript(`
local raw = redis.call('GET', KEYS[1])
if not raw or redis.call('PTTL', KEYS[1]) <= 0 then return 0 end
local ok, data = pcall(cjson.decode, raw)
if not ok or type(data) ~= 'table' or type(data.Token) ~= 'string' then return 0 end
local expected = ARGV[1]
if string.sub(data.Token, 1, 7) == 'sha256:' then expected = ARGV[2] end
local match = #data.Token == #expected
for i = 1, #data.Token do
  if string.byte(data.Token, i) ~= string.byte(expected, i) then match = false end
end
if not match then return 0 end
redis.call('DEL', KEYS[1])
return 1
`)

func (c *emailCache) verifyCode(ctx context.Context, key, code string, maxAttempts int) error {
	result, err := verifyEmailCodeScript.Run(ctx, c.rdb, []string{key}, code, maxAttempts).Int()
	if err != nil {
		return service.ErrInvalidVerifyCode
	}
	switch result {
	case 1:
		return nil
	case -2:
		return service.ErrVerifyCodeMaxAttempts
	default:
		return service.ErrInvalidVerifyCode
	}
}

func (c *emailCache) VerifyVerificationCode(ctx context.Context, email, code string, maxAttempts int) error {
	return c.verifyCode(ctx, verifyCodeKey(email), code, maxAttempts)
}

func (c *emailCache) VerifyNotifyVerificationCode(ctx context.Context, email, code string, maxAttempts int) error {
	return c.verifyCode(ctx, notifyVerifyKey(email), code, maxAttempts)
}

func (c *emailCache) ConsumePasswordResetToken(ctx context.Context, email, token, tokenHash string) (bool, error) {
	result, err := consumeEmailResetTokenScript.Run(ctx, c.rdb, []string{passwordResetKey(email)}, token, tokenHash).Int()
	return result == 1, err
}
