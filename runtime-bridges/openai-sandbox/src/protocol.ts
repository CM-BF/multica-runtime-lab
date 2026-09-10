export const SDK_VERSION = '0.17.2';
export const PROTOCOL = 1;
export type Request = {
  type: 'execute'; requestId: string; prompt: string; instructions: string; model: string;
  cwd: string; stateRoot: string; sessionId?: string; maxTurns?: number;
  inputs: string[]; artifacts: string[]; mcp?: {mcpServers: Record<string, MCPEntry>};
};
export type MCPEntry = {type?: string; command?: string; args?: string[]; env?: Record<string,string>; url?: string; headers?: Record<string,string>};
export type Frame = Record<string, unknown>;
export type Send = (frame: Frame) => Promise<void>;
export class BridgeError extends Error { constructor(public code: string) {super(code);} }
export function safeError(error: unknown): string {
  if (error instanceof BridgeError) return error.code;
  const e = error as {name?: string;status?: number;message?: string};
  if (e?.name === 'AbortError') return 'CANCELLED';
  if (e?.status === 401 || /missing credentials|api.?key.*required/i.test(e?.message ?? '')) return 'MODEL_CREDENTIALS';
  if (e?.status === 429) return 'MODEL_RATE_LIMIT';
  if (/max.?turn/i.test(e?.name ?? '')) return 'MAX_TURNS';
  return 'SDK_FAILURE';
}
export function mapEvent(event: any): Frame | undefined {
  if (event.type === 'raw_model_stream_event' && event.data?.type === 'output_text_delta') return {kind:'text',text:event.data.delta};
  if (event.type === 'agent_updated_stream_event') return {kind:'status',status:'agent_updated'};
  if (event.type !== 'run_item_stream_event') return;
  const item = event.item, raw = item?.rawItem;
  const id = raw?.callId ?? raw?.call_id ?? raw?.id;
  if (event.name === 'tool_called' || event.name === 'tool_output') {
    if (typeof id !== 'string') throw new BridgeError('INVALID_TOOL_EVENT');
    return {kind:event.name === 'tool_called' ? 'tool_use':'tool_result',id,name:raw?.name ?? raw?.type ?? 'sandbox',text:event.name === 'tool_output' ? String(item.output ?? '') : ''};
  }
  if (event.name === 'tool_approval_requested') return {kind:'status',status:'approval_required'};
  if (event.name.startsWith('handoff_')) return {kind:'status',status:event.name};
  // Final message items are not appended after text deltas (no duplicated answer).
}
