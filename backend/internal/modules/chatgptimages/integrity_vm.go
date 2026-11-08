package chatgptimages

import (
	"context"
	_ "embed"
	"encoding/base64"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/dop251/goja"
	"github.com/google/uuid"
)

//go:embed integrity_vm.js
var integrityVMSource string

var integrityVMProgram = sync.OnceValues(func() (*goja.Program, error) {
	return goja.Compile("chatgpt-integrity-vm", integrityVMSource, true)
})

func (i *Integrity) solveVM(ctx context.Context, dx string) (string, error) {
	if len(dx) > 128<<10 {
		return "", ErrVMBudget
	}
	if _, err := decodeIntegrityBase64(dx); err != nil {
		return "", ErrChallenge
	}
	program, err := integrityVMProgram()
	if err != nil {
		return "", ErrChallenge
	}
	vm := goja.New()
	vm.SetMaxCallStackSize(128)
	// No Go objects, require(), network, filesystem or process APIs are exported.
	// Only narrow scalar helpers are installed; the VM receives a JSON profile.
	bindings := map[string]any{
		"__now":  func() float64 { return float64(time.Since(i.started)) / float64(time.Millisecond) },
		"__uuid": func() (string, error) { id, err := uuid.NewRandom(); return id.String(), err },
		"__btoa": func(s string) (string, error) {
			if len(s) > 256<<10 {
				return "", ErrVMBudget
			}
			data := make([]byte, 0, len(s))
			for _, c := range s {
				if c > 255 {
					return "", errors.New("InvalidCharacterError")
				}
				data = append(data, byte(c))
			}
			return base64.StdEncoding.EncodeToString(data), nil
		},
		"__atob": func(s string) (string, error) {
			if len(s) > 256<<10 {
				return "", ErrVMBudget
			}
			data, err := decodeIntegrityBase64(s)
			if err != nil {
				return "", errors.New("InvalidCharacterError")
			}
			var result strings.Builder
			for _, b := range data {
				_, _ = result.WriteRune(rune(b))
			}
			return result.String(), nil
		},
	}
	for name, value := range bindings {
		if err := vm.Set(name, value); err != nil {
			return "", ErrChallenge
		}
	}
	start := time.Now()
	vmCtx, cancel := context.WithTimeout(ctx, 550*time.Millisecond)
	defer cancel()
	stop := context.AfterFunc(vmCtx, func() { vm.Interrupt(ErrVMBudget) })
	defer stop()
	value, err := vm.RunProgram(program)
	if err != nil {
		return "", ErrChallenge
	}
	object := value.ToObject(vm)
	run, ok := goja.AssertFunction(object.Get("run"))
	if !ok {
		return "", ErrChallenge
	}
	profile, err := integrityJSON(i.profile)
	if err != nil {
		return "", err
	}
	result, err := run(goja.Undefined(), vm.ToValue(string(profile)), vm.ToValue(dx), vm.ToValue(i.key))
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", ErrVMBudget
	}
	promise, ok := result.Export().(*goja.Promise)
	if !ok {
		return "", ErrChallenge
	}
	if promise.State() == goja.PromiseStatePending {
		timer := time.NewTimer(max(0, 500*time.Millisecond-time.Since(start)))
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-vmCtx.Done():
			return "", ErrVMBudget
		case <-timer.C:
		}
		// No external event sources exist in this runtime. The source's 500 ms
		// timeout resolves to the executed instruction count, not a fake proof.
		expire, ok := goja.AssertFunction(object.Get("expire"))
		if !ok {
			return "", ErrChallenge
		}
		if _, err := expire(goja.Undefined()); err != nil {
			return "", ErrVMBudget
		}
	}
	if promise.State() == goja.PromiseStateRejected {
		if strings.Contains(promise.Result().String(), "BudgetError") {
			return "", ErrVMBudget
		}
		return "", ErrVMRejected
	}
	if promise.State() != goja.PromiseStateFulfilled {
		return "", ErrVMBudget
	}
	token := promise.Result().String()
	if len(token) > 256<<10 {
		return "", ErrVMBudget
	}
	return token, nil
}

func decodeIntegrityBase64(s string) ([]byte, error) {
	s = strings.Map(func(c rune) rune {
		if c == ' ' || c == '\t' || c == '\r' || c == '\n' || c == '\f' {
			return -1
		}
		return c
	}, s)
	data, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return base64.RawStdEncoding.DecodeString(s)
	}
	return data, nil
}
