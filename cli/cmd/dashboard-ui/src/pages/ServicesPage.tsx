import { useApi } from '../api';
import type { K8sList, K8sService } from '../types';
import { StatusBadge, LabelBadges, EmptyState, TimeAgo } from './shared';

export function ServicesPage() {
  const { data, loading } = useApi<K8sList<K8sService>>('/api/services');

  if (loading) return <div className="loading">Loading services…</div>;

  const services = (data?.items || []).filter(
    (s) => s.metadata.namespace !== 'kube-system' && s.metadata.namespace !== 'local-path-storage'
  );

  return (
    <div className="page">
      <div className="page-header">
        <div className="page-header-left">
          <h1>Services</h1>
          <p className="page-subtitle">Cluster service endpoints</p>
        </div>
      </div>

      {services.length === 0 ? (
        <EmptyState icon="○" message="No services found in workload namespaces." />
      ) : (
        <div className="table-wrap">
          <table className="data-table">
            <thead>
              <tr>
                <th>Name</th>
                <th>Namespace</th>
                <th>Type</th>
                <th>Cluster IP</th>
                <th>Ports</th>
                <th>Selector</th>
                <th>Age</th>
              </tr>
            </thead>
            <tbody>
              {services.map((svc) => {
                const ports = svc.spec.ports?.map((p) => {
                  let s = `${p.port}`;
                  if (p.targetPort) s += `→${p.targetPort}`;
                  if (p.protocol && p.protocol !== 'TCP') s += `/${p.protocol}`;
                  if (p.nodePort) s += ` (node:${p.nodePort})`;
                  return s;
                }).join(', ') || '—';

                return (
                  <tr key={`${svc.metadata.namespace}/${svc.metadata.name}`}>
                    <td className="mono">{svc.metadata.name}</td>
                    <td><span className="tag">{svc.metadata.namespace}</span></td>
                    <td>
                      <StatusBadge
                        ok={svc.spec.type === 'ClusterIP' || svc.spec.type === 'LoadBalancer'}
                        label={svc.spec.type || 'ClusterIP'}
                      />
                    </td>
                    <td className="mono">{svc.spec.clusterIP || '—'}</td>
                    <td className="mono">{ports}</td>
                    <td>
                      {svc.spec.selector ? (
                        <LabelBadges labels={svc.spec.selector} />
                      ) : (
                        <span className="text-dim">—</span>
                      )}
                    </td>
                    <td><TimeAgo timestamp={svc.metadata.creationTimestamp} /></td>
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
