// Package devicekey calls the installed desktop's native device-key module.
// No private key bytes or software-key fallback cross this process boundary.
package devicekey

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/shared/remotecontrol"
)

const (
	HardwareOnly     = "hardware_only"
	AllowOSProtected = "allow_os_protected_nonextractable"
)

var ErrUnavailable = errors.New("native device key provider unavailable")

type Provider struct {
	NodePath   string
	ModulePath string
	Timeout    time.Duration
}

func NewProvider(resourcesPath string) *Provider {
	return &Provider{NodePath: filepath.Join(resourcesPath, "cua_node", "bin", "node"), ModulePath: filepath.Join(resourcesPath, "native", "remote-control-device-key.node"), Timeout: 35 * time.Second}
}

func (p *Provider) Check() error {
	if p == nil {
		return ErrUnavailable
	}
	for _, path := range []string{p.NodePath, p.ModulePath} {
		if !filepath.IsAbs(path) {
			return ErrUnavailable
		}
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() {
			return ErrUnavailable
		}
	}
	return nil
}

func (p *Provider) CreateDeviceKey(ctx context.Context, policy string) (remotecontrol.DeviceKeyPublic, error) {
	if policy != HardwareOnly && policy != AllowOSProtected {
		return remotecontrol.DeviceKeyPublic{}, errors.New("unsupported device key creation policy")
	}
	var key remotecontrol.DeviceKeyPublic
	if err := p.call(ctx, map[string]string{"operation": "create", "policy": policy}, &key); err != nil {
		return key, err
	}
	if _, err := remotecontrol.ParseDevicePublicKey(key); err != nil {
		return remotecontrol.DeviceKeyPublic{}, err
	}
	if policy == HardwareOnly && key.ProtectionClass == "os_protected_nonextractable" {
		_ = p.DeleteDeviceKey(context.WithoutCancel(ctx), key.KeyID)
		return remotecontrol.DeviceKeyPublic{}, errors.New("native key did not satisfy hardware-only policy")
	}
	return key, nil
}

func (p *Provider) GetDeviceKeyPublic(ctx context.Context, keyID string) (remotecontrol.DeviceKeyPublic, error) {
	var key remotecontrol.DeviceKeyPublic
	if err := p.call(ctx, map[string]string{"operation": "public", "keyId": keyID}, &key); err != nil {
		return key, err
	}
	if key.KeyID != keyID {
		return remotecontrol.DeviceKeyPublic{}, errors.New("native key identity mismatch")
	}
	_, err := remotecontrol.ParseDevicePublicKey(key)
	return key, err
}

func (p *Provider) DeleteDeviceKey(ctx context.Context, keyID string) error {
	return p.call(ctx, map[string]string{"operation": "delete", "keyId": keyID}, nil)
}

func (p *Provider) SignDeviceKey(ctx context.Context, keyID string, payload []byte) (remotecontrol.DeviceSignature, error) {
	if len(payload) == 0 || len(payload) > 16*1024 {
		return remotecontrol.DeviceSignature{}, errors.New("invalid device signing payload size")
	}
	var signature remotecontrol.DeviceSignature
	err := p.call(ctx, map[string]string{"operation": "sign", "keyId": keyID, "payload": base64.StdEncoding.EncodeToString(payload)}, &signature)
	return signature, err
}

type boundedOutput struct {
	buffer   bytes.Buffer
	exceeded bool
}

func (b *boundedOutput) Write(data []byte) (int, error) {
	if b.buffer.Len()+len(data) > 32*1024 {
		b.exceeded = true
		return 0, errors.New("native device key output exceeds limit")
	}
	return b.buffer.Write(data)
}

func (p *Provider) call(ctx context.Context, input map[string]string, output any) error {
	if err := p.Check(); err != nil {
		return err
	}
	if id := input["keyId"]; input["operation"] != "create" && (id == "" || len(id) > 256) {
		return errors.New("invalid device key reference")
	}
	timeout := p.Timeout
	if timeout <= 0 || timeout > time.Minute {
		timeout = 35 * time.Second
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	payload, err := json.Marshal(input)
	if err != nil {
		return err
	}
	command := exec.CommandContext(runCtx, p.NodePath, "-e", nativeScript, p.ModulePath)
	command.Stdin = bytes.NewReader(payload)
	var stdout boundedOutput
	command.Stdout = &stdout
	command.Stderr = io.Discard
	if err := command.Run(); err != nil {
		if runCtx.Err() != nil {
			return runCtx.Err()
		}
		return ErrUnavailable
	}
	if stdout.exceeded {
		return ErrUnavailable
	}
	if output == nil {
		return nil
	}
	if err := json.Unmarshal(stdout.buffer.Bytes(), output); err != nil {
		return errors.New("malformed native device key response")
	}
	return nil
}

const nativeScript = `
const addon = require(process.argv[1]);
const fs = require('node:fs');
(async () => {
  const request = JSON.parse(fs.readFileSync(0, 'utf8'));
  let result;
  switch (request.operation) {
    case 'create': result = await addon.createDeviceKey(request.policy); break;
    case 'public': result = await addon.getDeviceKeyPublic(request.keyId); break;
    case 'delete': await addon.deleteDeviceKey(request.keyId); result = {}; break;
    case 'sign': result = await addon.signDeviceKey(request.keyId, Buffer.from(request.payload, 'base64')); break;
    default: throw new Error('unsupported device key operation');
  }
  process.stdout.write(JSON.stringify(result));
})().catch(() => { process.exitCode = 1; });
`
