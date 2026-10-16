package provider

import (
    "context"
    "fmt"
    "net/http"
    "net/http/httptest"
    "testing"
)

func TestUpstreamSyncTransactionProbe(t *testing.T) {
    act := ""
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        act = r.URL.Query().Get("act")
        _, _ = w.Write([]byte(`{"code":"1","status":"1","money":10,"trade_no":98765}`))
    }))
    defer server.Close()
    p := newTestEasyPay(t, server.URL)
    result, err := p.QueryOrder(context.Background(), "probe-order")
    if err != nil {
        fmt.Printf("OBSERVED easypay_query_act=%q result=ERROR\n", act)
        return
    }
    fmt.Printf("OBSERVED easypay_query_act=%q status=%s amount=%.0f trade_no=%s\n", act, result.Status, result.Amount, result.TradeNo)
}

func BenchmarkUpstreamSyncEasyPayQuery(b *testing.B) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        _, _ = w.Write([]byte(`{"code":1,"status":1,"money":"10.00","trade_no":"98765"}`))
    }))
    defer server.Close()
    p, err := NewEasyPay("benchmark", map[string]string{
        "pid": "fixture", "pkey": "fixture", "apiBase": server.URL,
        "notifyUrl": "https://fixture.invalid/notify", "returnUrl": "https://fixture.invalid/return",
    })
    if err != nil { b.Fatal(err) }
    b.ReportAllocs()
    b.ResetTimer()
    for b.Loop() {
        result, err := p.QueryOrder(context.Background(), "probe-order")
        if err != nil || result == nil || result.Amount != 10 { b.Fatal(result, err) }
    }
}
