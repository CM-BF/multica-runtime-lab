// @vitest-environment node
import { describe, expect, it } from "vitest";
import { buildRuntimeCatalog, PROTOCOL_FAMILIES, parseCommandLine } from "./runtime-profile-catalog";
import { providerDisplayName } from "@multica/core/runtimes";
import { RUNTIME_PROFILE_PROTOCOL_FAMILIES } from "@multica/core/types";

describe("Deep Agents catalog", () => {
  it("exposes a selectable family with the official entry and display name", () => {
    expect(RUNTIME_PROFILE_PROTOCOL_FAMILIES).toContain("deepagents");
    expect(PROTOCOL_FAMILIES).toContain("deepagents");
    expect(buildRuntimeCatalog([]).builtins).toContainEqual({kind:"builtin", id:"builtin:deepagents", protocolFamily:"deepagents"});
    expect(providerDisplayName("deepagents")).toBe("Deep Agents");
    expect(parseCommandLine('"/isolated path/bin/dcode"')).toEqual({ok:true,commandName:"/isolated path/bin/dcode",fixedArgs:[]});
  });
});
