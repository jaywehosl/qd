import { useEffect, useSyncExternalStore } from 'react';

import { useStatusQuery } from '@/api/queries/useStatusQuery';
import { subscribe, getSnapshot, clearAckSensor } from '@/stores/notificationStore';
import { evalStatusSensors } from '@/lib/notifications/statusSensors';

/**
 * Status sensors are LIVE conditions: useNotifications renders/updates/clears
 * their rows directly off the polled status. The only thing that needs a
 * side-effect is RE-ARMING — when a sensor the user dismissed for the current
 * episode drops back below its threshold, clear its ack so the next spike
 * alerts again. Mounted once in PanelLayout.
 */
export default function SensorWatcher() {
  const { status, fetched } = useStatusQuery();
  const { sensors, sensorAcked } = useSyncExternalStore(subscribe, getSnapshot, getSnapshot);

  useEffect(() => {
    if (!fetched || sensorAcked.length === 0) return;
    for (const s of evalStatusSensors(status, sensors)) {
      if (!s.over && sensorAcked.includes(s.key)) clearAckSensor(s.key);
    }
  }, [status, fetched, sensors, sensorAcked]);

  return null;
}
