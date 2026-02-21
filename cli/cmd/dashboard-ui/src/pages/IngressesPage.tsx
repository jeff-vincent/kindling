import { useApi } from '../api';
import type { K8sList, K8sIngress } from '../types';
import { StatusBadge, EmptyState, TimeAgo } from './shared';

export function IngressesPage() {
  const { data, loading } = useApi<K8sList<K8sIngress>>('/api/ingresses');

  if (loading) return <div className="loading">Loading ingresses…</div>;

  const ingresses = data?.items || [];

  return (
    <div className="page">
      <div className="page-header">
        <div className="page-header-left">
          <h1>Ingresses</h1>
          <p className="page-subtitle">External access routing rules</p>
        </div>
      </div>

      {ingresses.length === 0 ? (
        <EmptyState icon="◎" message="No ingresses configured." />
      ) : (
        <div className="table-wrap">
          <table className="data-table">
            <thead>
              <tr>
                <th>Name</th>
                <th>Namespace</th>
                <th>Class</th>
                <th>Hosts</th>
                <th>Paths</th>
                <th>Status</th>
                <th>Age</th>
              </tr>
            </thead>
            <tbody>
              {ingresses.map((ing) => {
                const rules = ing.spec.rules || [];
                const hosts = rules.map((r) => r.host || '*').join(', ') || '—';
                const paths = rules.flatMap((r) =>
                  (r.http?.paths || []).map((p) => {
                    const svc = p.backend?.service;
                    const pathStr = p.path || '/';
                    const svcStr = svc ? `${svc.name}:${svc.port?.number || svc.port?.name || '?'}` : '?';
                    return `${pathStr} → ${svcStr}`;
                  })
                );
                const hasIP = ing.status?.loadBalancer?.ingress?.some((i: any) => i.ip || i.hostname);

                return (
                  <tr key={`${ing.metadata.namespace}/${ing.metadata.name}`}>
                    <td className="mono">{ing.metadata.name}</td>
                    <td><span className="tag">{ing.metadata.namespace}</span></td>
                    <td className="mono">{ing.spec.ingressClassName || '—'}</td>
                    <td className="mono">{hosts}</td>
                    <td>
                      {paths.length > 0 ? (
                        <div style={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
                          {paths.map((p, i) => (
                            <span key={i} className="mono" style={{ fontSize: '0.8em' }}>{p}</span>
                          ))}
                        </div>
                      ) : (
                        <span className="text-dim">—</span>
                      )}
                    </td>
                    <td>
                      <StatusBadge ok={!!hasIP} label={hasIP ? 'Active' : 'Pending'} />
                    </td>
                    <td><TimeAgo timestamp={ing.metadata.creationTimestamp} /></td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
