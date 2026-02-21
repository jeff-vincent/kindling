import { useApi } from '../api';
import type { K8sList, K8sEvent } from '../types';
import { EmptyState, TimeAgo } from './shared';

export function EventsPage() {
  const { data, loading } = useApi<K8sList<K8sEvent>>('/api/events');

  if (loading) return <div className="loading">Loading events…</div>;

  const events = (data?.items || []).filter(
    (e) => e.metadata.namespace !== 'kube-system' && e.metadata.namespace !== 'local-path-storage'
  );

  // Sort by last timestamp descending
  events.sort((a, b) => {
    const ta = a.lastTimestamp || a.metadata.creationTimestamp || '';
    const tb = b.lastTimestamp || b.metadata.creationTimestamp || '';
    return tb.localeCompare(ta);
  });

  return (
    <div className="page">
      <div className="page-header">
        <div className="page-header-left">
          <h1>Events</h1>
          <p className="page-subtitle">Recent cluster events</p>
        </div>
      </div>

      {events.length === 0 ? (
        <EmptyState icon="◈" message="No events in workload namespaces." />
      ) : (
        <div className="table-wrap">
          <table className="data-table">
            <thead>
              <tr>
                <th style={{ width: 72 }}>Type</th>
                <th>Reason</th>
                <th>Object</th>
                <th>Namespace</th>
                <th>Message</th>
                <th style={{ width: 60 }}>Count</th>
                <th>Last Seen</th>
              </tr>
            </thead>
            <tbody>
              {events.map((ev, i) => (
                <tr key={`${ev.metadata.namespace}/${ev.metadata.name}-${i}`}>
                  <td>
                    <span className={`event-badge event-${(ev.type || 'Normal').toLowerCase()}`}>
                      {ev.type || 'Normal'}
                    </span>
                  </td>
                  <td className="mono">{ev.reason}</td>
                  <td className="mono" style={{ fontSize: '0.8em' }}>
                    {ev.involvedObject?.kind}/{ev.involvedObject?.name}
                  </td>
                  <td><span className="tag">{ev.metadata.namespace}</span></td>
                  <td style={{ maxWidth: 400, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                    {ev.message}
                  </td>
                  <td style={{ textAlign: 'center' }}>{ev.count || 1}</td>
                  <td><TimeAgo timestamp={ev.lastTimestamp || ev.metadata.creationTimestamp} /></td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
