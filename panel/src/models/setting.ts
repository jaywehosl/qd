import { ObjectUtil } from '@/utils';

export class AllSetting {
  pageSize = 25;
  refreshMinutes: number = 480;
  blockTier = 'pro';
  blockTif = false;
  expireDiff = 0;
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
  bbrProfile = 'standard';
  ipv6 = false;
  maxStreams = 65536;
  streamWindowKb = 512;
  maxStreamWindowKb = 16384;
  connWindowKb = 98304;
  maxConnWindowKb = 98304;
  idleSeconds = 90;
  keepAliveSeconds = 15;
  socketBufferKb = 2048;
  echName = '';
  clientVersion = '';

  constructor(data?: unknown) {
    if (data != null) {
      ObjectUtil.cloneProps(this, data);
    }
  }

  equals(other: AllSetting): boolean {
    return ObjectUtil.equals(this, other);
  }
}
