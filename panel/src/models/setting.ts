import { ObjectUtil } from '@/utils';

export class AllSetting {
  pageSize = 25;
  refreshMinutes: number = 480;
  expireDiff = 0;
  trafficDiff = 0;
  remarkModel = '-io';
  datepicker: 'gregorian' | 'jalalian' = 'gregorian';
  timeLocation = 'Local';
  dnsPrimary = '1.1.1.1';
  dnsSecondary = '8.8.8.8';
  dnsCache = 4096;
  dnsMinTtl = 60;
  dnsMaxTtl = 3600;
  dnsStale = 60;
  mtu = 1500;
  statsSeconds = 5;
  pool = '10.7.0.0/16';
  brutalMbit = 0;
  maxStreams = 65536;
  streamWindowKb = 2048;
  maxStreamWindowKb = 6144;
  connWindowKb = 3072;
  maxConnWindowKb = 15360;
  idleSeconds = 90;
  keepAliveSeconds = 15;
  socketBufferKb = 2048;

  constructor(data?: unknown) {
    if (data != null) {
      ObjectUtil.cloneProps(this, data);
    }
  }

  equals(other: AllSetting): boolean {
    return ObjectUtil.equals(this, other);
  }
}
