import type { Device } from '@/shared/api/types';
import { RevokeDeviceButton } from '@/features/device-revoke';
import { formatDateTime } from '@/shared/lib/format';
import { Badge, Card } from '@/shared/ui';

/** Устройства с активной сессией; у каждого — «Завершить сеанс» (фича device-revoke). */
export function DeviceList({ items }: { items: Device[] }) {
  return (
    <div className="stack">
      {items.map((d) => (
        <Card key={d.id} as="article" className="row-between">
          <div>
            <b>{d.user_agent}</b> {d.current && <Badge tone="ok">Это устройство</Badge>}
            <div className="muted">
              {d.ip} · вход {formatDateTime(d.created_at)} · активность {formatDateTime(d.last_seen_at)}
            </div>
          </div>
          <RevokeDeviceButton id={d.id} current={d.current} />
        </Card>
      ))}
    </div>
  );
}
