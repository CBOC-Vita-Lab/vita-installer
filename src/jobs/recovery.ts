import type { Transport } from '../devices/transport';
import type { FileJournal } from './journal';
import type { JobSnapshot } from '../shared/contracts';
import { assertBinding } from './binding';
import { executeTransfer } from './transfer';
export async function recoverJob(jobId: string, transport: Transport, journal: FileJournal,
  onProgress: (snapshot: JobSnapshot) => void, signal?: AbortSignal): Promise<JobSnapshot> {
  const record = await journal.load(jobId); await assertBinding(record, transport, signal);
  if (record.promotion !== 'not_started') {
    record.snapshot = { ...record.snapshot, phase: 'paused', completionMode: 'needs_attention', errorCode: 'promotion_needs_reconciliation', availableActions: ['retry', 'cancel'] };
    await journal.save(record); onProgress(record.snapshot); return record.snapshot;
  }
  return executeTransfer(record.plan, transport, journal, onProgress, signal);
}
