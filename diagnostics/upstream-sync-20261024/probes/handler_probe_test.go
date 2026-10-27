package handler
import (
 "fmt"
 "reflect"
 "testing"
 "time"
 "github.com/Wei-Shaw/sub2api/internal/application/service"
)
func TestUpstreamSync20261024Probe(t *testing.T){
 e:=&service.UpstreamFailoverError{Scope:service.GatewayFailureScopeRequest,RetryableOnSameAccount:true}
 f:=reflect.ValueOf(e).Elem().FieldByName("SameAccountRetryDelay");if f.IsValid(){f.SetInt(int64(6250*time.Millisecond))}
 for _,n:=range []int{1,2,5}{fmt.Printf("HINTED_RETRY_%d=%s\n",n,sameAccountRetryDelayFor(e,n))}
}
