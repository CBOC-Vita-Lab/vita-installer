import type { InstallPlan } from '../jobs/types';
import type { InstallationServices } from '../jobs/install';
import { openTransport, stageRoot, type Transport } from './transport';
import { inspectDevice } from './native';
import { promoteCompanion } from './companion';
import { requireThat } from '../shared/errors';
export function nativeInstallationServices(plan: InstallPlan, transport: Transport): InstallationServices {
  return {
    inspect: inspectDevice,
    promote: async (device, path) => {
      requireThat(device.identity === plan.target.identity && device.identity === await transport.identity(), 'device_changed');
      requireThat(device.state === 'ready' && device.transport === 'ftp' && device.capabilities.install, 'promotion_not_supported');
      requireThat(path === (device.adapter === 'companion' ? `${stageRoot(plan.jobId)}/app` : plan.vpkDestination), 'promotion_path_mismatch');
      if (device.adapter === 'companion') return promoteCompanion(device, plan.jobId);
      requireThat(device.adapter === 'vitashell' && transport.promoteVpk, 'promotion_not_supported');
      return transport.promoteVpk();
    },
    reconnect: async () => {
      const current = await inspectDevice(plan.target);
      requireThat(current.state === 'ready' && current.identity === plan.target.identity && current.transport === 'ftp', 'device_changed');
      return openTransport(current);
    }
  };
}
