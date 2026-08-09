package prompts

import "embed"

// The non-overridable output contract blocks. One per agent: the JSON
// envelope schema and the tool list for the role. They are always appended
// by code after rendering the (possibly user-overridden) action body, so a
// custom template can never break the machine-readable protocol that the
// orchestrator depends on. There is deliberately no override or append path
// for these blocks.
//
//go:embed contracts/*.txt
var contractsFS embed.FS

// Contract returns the non-overridable output contract block for the given
// agent. It panics on unknown agents: the catalog is the compile-time source
// of truth, and every catalog agent ships a contract file.
func Contract(agent string) string {
	data, err := contractsFS.ReadFile("contracts/" + agent + ".txt")
	if err != nil {
		panic("prompts: missing output contract for agent " + agent + ": " + err.Error())
	}
	return string(data)
}
