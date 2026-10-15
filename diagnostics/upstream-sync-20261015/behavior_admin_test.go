//go:build unit
package admin
import("fmt";"testing")
func TestUpstreamSyncBehavior(t *testing.T){
 a:=buildCodexStoredIdentityKeys("workspace-a","same-user","fixture@example.com","");b:=buildCodexStoredIdentityKeys("workspace-b","same-user","fixture@example.com","")
 overlap:=false;for _,x:=range a{for _,y:=range b{overlap=overlap||x==y}}
 fmt.Printf("OBSERVED_ADMIN cross_workspace_identity_overlap=%t\n",overlap)
}
