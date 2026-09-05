import { useEffect, useSyncExternalStore } from 'react';

import { useStatusQuery } from '@/api/queries/useStatusQuery';
import { subscribe, getSnapshot, clearAckSensor } from '@/stores/notificationStore';
import { evalStatusSensors } from '@/lib/notifications/statusSensors';

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
