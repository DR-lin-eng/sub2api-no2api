"""Compare the two actual history repair functions with identical Docker inputs."""
from pathlib import Path
import shutil
import subprocess
import tempfile

repo = Path(__file__).resolve().parents[2]
backend = repo / "backend"
service = backend / "internal/application/service"
name = "openai_gateway_deepseek_anthropic_thinking.go"
upstream = subprocess.check_output(
    ["git", "show", "50dc3141e7dea62f2ed3b92998c1ac4367172f1a:backend/internal/service/" + name],
    cwd=repo, text=True)


def function(source, name):
    start = source.index("func " + name + "(")
    return source[start:source.index("\n}", start) + 2]


with tempfile.TemporaryDirectory() as tmp:
    work = Path(tmp)
    for file in ["go.mod", "go.sum"]:
        shutil.copy(backend / file, work)
    function_name = "ensureDeepSeekAnthropicThinkingBlocks"
    original = function(upstream, function_name).replace(function_name, "frozenUpstream")
    candidate = function((service / name).read_text(), function_name)
    raw_view = function((service / "gateway_service.go").read_text(), "parseRawJSONView")
    source = '''package bench
import ("bytes"; "fmt"; "strconv"; "strings"; "testing"; "unsafe"
 "github.com/tidwall/gjson"; "github.com/tidwall/sjson")
const deepSeekAnthropicThinkingPlaceholderBlock = `{"type":"thinking","thinking":" "}`
''' + original + "\n" + candidate + "\n" + raw_view + '''
func BenchmarkRepair(b *testing.B) {
 for _, turns := range []int{100,1000} {
  msg := `{"role":"assistant","content":[{"type":"tool_use","id":"use","input":{}}]}`
  input := []byte(`{"messages":[` + strings.TrimSuffix(strings.Repeat(msg+",",turns),",") + "]}")
  want:=frozenUpstream(input)
  if !bytes.Equal(want, ensureDeepSeekAnthropicThinkingBlocks(input)) {b.Fatal("output mismatch")}
  for _, algorithm := range []struct{name string; run func([]byte) []byte}{
   {"upstream",frozenUpstream},{"candidate",ensureDeepSeekAnthropicThinkingBlocks},
  } {
   b.Run(fmt.Sprintf("%s/%d",algorithm.name,turns),func(b *testing.B){
    b.ReportAllocs()
    for b.Loop() { _=algorithm.run(input) }
   })
  }
 }
}
'''
    (work / "repair_test.go").write_text(source)
    subprocess.run(["docker", "run", "--rm", "--cpus=1", "--memory=1g",
                    "-v", str(work) + ":/bench:ro", "-w", "/bench",
                    "-v", "sub2api-go-mod-cache:/go/pkg/mod",
                    "-v", "sub2api-go-build-cache:/root/.cache/go-build",
                    "-e", "GOMAXPROCS=1", "golang:1.26.11-alpine", "go", "test",
                    "-run", "^$", "-bench", "BenchmarkRepair", "-count=3",
                    "-benchtime=300ms"], check=True)
