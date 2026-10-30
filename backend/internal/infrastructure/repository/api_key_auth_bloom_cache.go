package repository

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const (
	authKeyBloomRedisKey    = "{apikey:membership:v1}:bitmap"
	authKeyBloomLeaseKey    = "{apikey:membership:v1}:builder"
	authKeyBloomBits        = 1 << 24 // 2 MiB shared per deployment, never per random credential.
	authKeyBloomHeaderBytes = 37      // B/R plus UUID; same Redis key as the bits.
	authKeyBloomBytes       = authKeyBloomBits/8 + authKeyBloomHeaderBytes
)

var errAuthKeyBloomNotReady = errors.New("API key membership filter is not ready")

// All readiness and bits live in a single string: eviction cannot leave a
// ready marker paired with an empty bitmap. No RedisBloom module is needed.
// Shared pre-insert registration creates only an unready bitmap if it is lost.
var authKeyBloomAddScript = redis.NewScript(`
local n=tonumber(ARGV[1])
local size=redis.call('STRLEN',KEYS[1])
if size==0 then
 redis.call('SET',KEYS[1],'B'..ARGV[2]..string.rep('\0',n-37))
elseif size~=n then
	-- A stale/partially evicted v1 value is recoverable. Recreate it as
	-- unready before registering the new credential; readers fail open until
	-- the next complete database scan publishes R.
	redis.call('SET',KEYS[1],'B'..ARGV[2]..string.rep('\0',n-37))
end
for i=3,#ARGV do redis.call('SETBIT',KEYS[1],tonumber(ARGV[i]),1) end
return 1
`)

var authKeyBloomContainsScript = redis.NewScript(`
if redis.call('STRLEN',KEYS[1])~=tonumber(ARGV[1]) or redis.call('GETRANGE',KEYS[1],0,0)~='R' then return -1 end
for i=2,#ARGV do
 if redis.call('GETBIT',KEYS[1],tonumber(ARGV[i]))==0 then return 0 end
end
return 1
`)

var authKeyBloomBeginScript = redis.NewScript(`
local size=redis.call('STRLEN',KEYS[1])
if size==tonumber(ARGV[1]) and redis.call('GETRANGE',KEYS[1],0,0)=='R' then return {'ready',''} end
if not redis.call('SET',KEYS[2],ARGV[2],'NX','PX',30000) then return {'busy',''} end
if size==0 then
 redis.call('SET',KEYS[1],'B'..ARGV[2]..string.rep('\0',tonumber(ARGV[1])-37))
elseif size~=tonumber(ARGV[1]) then
	redis.call('DEL',KEYS[1])
	redis.call('SET',KEYS[1],'B'..ARGV[2]..string.rep('\0',tonumber(ARGV[1])-37))
end
return {'build',redis.call('GETRANGE',KEYS[1],1,36)}
`)

var authKeyBloomBatchScript = redis.NewScript(`
if redis.call('GET',KEYS[2])~=ARGV[1] or redis.call('STRLEN',KEYS[1])~=tonumber(ARGV[3]) or redis.call('GETRANGE',KEYS[1],1,36)~=ARGV[2] then return 0 end
redis.call('PEXPIRE',KEYS[2],30000)
for i=4,#ARGV do redis.call('SETBIT',KEYS[1],tonumber(ARGV[i]),1) end
return 1
`)

var authKeyBloomFinishScript = redis.NewScript(`
if redis.call('GET',KEYS[2])~=ARGV[1] or redis.call('STRLEN',KEYS[1])~=tonumber(ARGV[3]) or redis.call('GETRANGE',KEYS[1],1,36)~=ARGV[2] then return 0 end
redis.call('SETRANGE',KEYS[1],0,'R')
redis.call('DEL',KEYS[2])
return 1
`)

var authKeyBloomReleaseScript = redis.NewScript(`if redis.call('GET',KEYS[1])==ARGV[1] then return redis.call('DEL',KEYS[1]) end return 0`)

func authKeyBloomOffsets(digest [sha256.Size]byte) [6]uint32 {
	a := binary.LittleEndian.Uint64(digest[:8])
	b := binary.LittleEndian.Uint64(digest[8:16]) | 1
	var offsets [6]uint32
	for i := range offsets {
		offsets[i] = uint32((a+uint64(i)*b)&(authKeyBloomBits-1)) + authKeyBloomHeaderBytes*8
	}
	return offsets
}

func (c *apiKeyCache) MayContainAuthKey(ctx context.Context, digest [sha256.Size]byte) (bool, error) {
	args := []any{authKeyBloomBytes}
	for _, bit := range authKeyBloomOffsets(digest) {
		args = append(args, bit)
	}
	result, err := authKeyBloomContainsScript.Run(ctx, c.rdb, []string{authKeyBloomRedisKey}, args...).Int()
	if err != nil {
		return true, err
	}
	if result < 0 {
		return true, errAuthKeyBloomNotReady
	}
	return result != 0, nil
}

func (c *apiKeyCache) RegisterAuthKey(ctx context.Context, key string) error {
	args := []any{authKeyBloomBytes, uuid.NewString()}
	for _, bit := range authKeyBloomOffsets(sha256.Sum256([]byte(key))) {
		args = append(args, bit)
	}
	return authKeyBloomAddScript.Run(ctx, c.rdb, []string{authKeyBloomRedisKey}, args...).Err()
}

func (c *apiKeyCache) InitializeAuthKeyBloom(ctx context.Context, load func(context.Context, func([][sha256.Size]byte) error) error) error {
	token := uuid.NewString()
	result, err := authKeyBloomBeginScript.Run(ctx, c.rdb, []string{authKeyBloomRedisKey, authKeyBloomLeaseKey}, authKeyBloomBytes, token).StringSlice()
	if err != nil {
		return err
	}
	if len(result) != 2 {
		return errAuthKeyBloomNotReady
	}
	if result[0] == "ready" {
		return nil
	}
	if result[0] != "build" {
		return errAuthKeyBloomNotReady
	}
	generation := result[1]
	defer func() {
		releaseCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = authKeyBloomReleaseScript.Run(releaseCtx, c.rdb, []string{authKeyBloomLeaseKey}, token).Err()
	}()
	err = load(ctx, func(digests [][sha256.Size]byte) error {
		args := []any{token, generation, authKeyBloomBytes}
		for _, digest := range digests {
			for _, bit := range authKeyBloomOffsets(digest) {
				args = append(args, bit)
			}
		}
		ok, err := authKeyBloomBatchScript.Run(ctx, c.rdb, []string{authKeyBloomRedisKey, authKeyBloomLeaseKey}, args...).Int()
		if err != nil {
			return err
		}
		if ok != 1 {
			return errAuthKeyBloomNotReady
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("warm API key membership: %w", err)
	}
	ok, err := authKeyBloomFinishScript.Run(ctx, c.rdb, []string{authKeyBloomRedisKey, authKeyBloomLeaseKey}, token, generation, authKeyBloomBytes).Int()
	if err != nil {
		return err
	}
	if ok != 1 {
		return errAuthKeyBloomNotReady
	}
	return nil
}
