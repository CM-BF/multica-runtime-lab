// @vitest-environment node
import { describe, expect, it } from "vitest";
import { buildRuntimeCatalog, PROTOCOL_FAMILIES, parseCommandLine } from "./runtime-profile-catalog";
import { providerDisplayName } from "@multica/core/runtimes";

describe("OpenAI Sandbox catalog", () => {
  it("exposes the installed bridge as a selectable runtime family", () => {
    expect(PROTOCOL_FAMILIES).toContain("openai-sandbox");
    expect(buildRuntimeCatalog([]).builtins).toContainEqual({kind:"builtin",id:"builtin:openai-sandbox",protocolFamily:"openai-sandbox"});
    expect(providerDisplayName("openai-sandbox")).toBe("OpenAI Sandbox");
    expect(parseCommandLine('"/private path/bin/multica-openai-sandbox"')).toEqual({ok:true,commandName:"/private path/bin/multica-openai-sandbox",fixedArgs:[]});
  });
});
