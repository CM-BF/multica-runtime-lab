package daemon
import("testing";"regexp";"github.com/multica-ai/multica/server/pkg/remotemcp")
func TestDA7BrokerNames(t *testing.T){
 a:=remotemcp.Connection{ContributionKey:"toolbox",ContributionID:"plugin:01a08abc-1111-7111-8111-111111111111:toolbox"}
 b:=a;b.ContributionID="plugin:01b09def-2222-7222-8222-222222222222:toolbox"
 t.Run("valid-plugin-name",func(t *testing.T){n:=remoteMCPServerName(a);if !regexp.MustCompile(`^[a-zA-Z0-9_-]+$`).MatchString(n){t.Fatal(n)};t.Log(n)})
 t.Run("distinct-installations",func(t *testing.T){x,y:=remoteMCPServerName(a),remoteMCPServerName(b);t.Logf("name1=%s name2=%s",x,y);if x==y{t.Fatal("distinct plugin installations collide; servers[name] overwrites earlier connection")}})
}
