import { useState } from 'react';
import { useApi } from '../api';
import type {
  K8sList,
  K8sServiceAccount,
  K8sRole,
  K8sRoleBinding,
  K8sClusterRole,
  K8sClusterRoleBinding,
} from '../types';
import { EmptyState, TimeAgo } from './shared';

type RBACTab = 'serviceaccounts' | 'roles' | 'rolebindings' | 'clusterroles' | 'clusterrolebindings';

const TABS: { key: RBACTab; label: string }[] = [
  { key: 'serviceaccounts', label: 'Service Accounts' },
  { key: 'roles', label: 'Roles' },
  { key: 'rolebindings', label: 'Role Bindings' },
  { key: 'clusterroles', label: 'Cluster Roles' },
  { key: 'clusterrolebindings', label: 'Cluster Role Bindings' },
];

export function RBACPage() {
  const [tab, setTab] = useState<RBACTab>('serviceaccounts');

  return (
    <div className="page">
      <div className="page-header">
        <div className="page-header-left">
          <h1>RBAC</h1>
          <p className="page-subtitle">Role-based access control resources</p>
        </div>
      </div>

      <div className="tab-bar">
        {TABS.map((t) => (
          <button
            key={t.key}
            className={`tab-btn${tab === t.key ? ' tab-active' : ''}`}
            onClick={() => setTab(t.key)}
          >
            {t.label}
          </button>
        ))}
      </div>

      {tab === 'serviceaccounts' && <ServiceAccountsTab />}
      {tab === 'roles' && <RolesTab />}
      {tab === 'rolebindings' && <RoleBindingsTab />}
      {tab === 'clusterroles' && <ClusterRolesTab />}
      {tab === 'clusterrolebindings' && <ClusterRoleBindingsTab />}
    </div>
  );
}

// ── Service Accounts ────────────────────────────────────────────

function ServiceAccountsTab() {
  const { data, loading } = useApi<K8sList<K8sServiceAccount>>('/api/serviceaccounts');

  if (loading) return <div className="loading">Loading service accounts…</div>;

  const items = (data?.items || []).filter(
    (sa) =>
      sa.metadata.namespace !== 'kube-system' &&
      sa.metadata.namespace !== 'local-path-storage'
  );

  if (items.length === 0) {
    return <EmptyState icon="⊞" message="No service accounts found in workload namespaces." />;
  }

  return (
    <div className="table-wrap">
      <table className="data-table">
        <thead>
          <tr>
            <th>Name</th>
            <th>Namespace</th>
            <th>Secrets</th>
            <th>Age</th>
          </tr>
        </thead>
        <tbody>
          {items.map((sa) => (
            <tr key={`${sa.metadata.namespace}/${sa.metadata.name}`}>
              <td className="mono">{sa.metadata.name}</td>
              <td><span className="tag">{sa.metadata.namespace}</span></td>
              <td className="mono">{sa.secrets?.length ?? 0}</td>
              <td><TimeAgo timestamp={sa.metadata.creationTimestamp} /></td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

// ── Roles ───────────────────────────────────────────────────────

function RolesTab() {
  const { data, loading } = useApi<K8sList<K8sRole>>('/api/roles');

  if (loading) return <div className="loading">Loading roles…</div>;

  const items = (data?.items || []).filter(
    (r) =>
      r.metadata.namespace !== 'kube-system' &&
      r.metadata.namespace !== 'local-path-storage'
  );

  if (items.length === 0) {
    return <EmptyState icon="⊘" message="No roles found in workload namespaces." />;
  }

  return (
    <div className="table-wrap">
      <table className="data-table">
        <thead>
          <tr>
            <th>Name</th>
            <th>Namespace</th>
            <th>Rules</th>
            <th>Age</th>
          </tr>
        </thead>
        <tbody>
          {items.map((role) => (
            <tr key={`${role.metadata.namespace}/${role.metadata.name}`}>
              <td className="mono">{role.metadata.name}</td>
              <td><span className="tag">{role.metadata.namespace}</span></td>
              <td>{role.rules?.length ?? 0}</td>
              <td><TimeAgo timestamp={role.metadata.creationTimestamp} /></td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

// ── Role Bindings ───────────────────────────────────────────────

function RoleBindingsTab() {
  const { data, loading } = useApi<K8sList<K8sRoleBinding>>('/api/rolebindings');

  if (loading) return <div className="loading">Loading role bindings…</div>;

  const items = (data?.items || []).filter(
    (rb) =>
      rb.metadata.namespace !== 'kube-system' &&
      rb.metadata.namespace !== 'local-path-storage'
  );

  if (items.length === 0) {
    return <EmptyState icon="⊘" message="No role bindings found in workload namespaces." />;
  }

  return (
    <div className="table-wrap">
      <table className="data-table">
        <thead>
          <tr>
            <th>Name</th>
            <th>Namespace</th>
            <th>Role</th>
            <th>Subjects</th>
            <th>Age</th>
          </tr>
        </thead>
        <tbody>
          {items.map((rb) => (
            <tr key={`${rb.metadata.namespace}/${rb.metadata.name}`}>
              <td className="mono">{rb.metadata.name}</td>
              <td><span className="tag">{rb.metadata.namespace}</span></td>
              <td className="mono">{rb.roleRef.kind}/{rb.roleRef.name}</td>
              <td>
                {rb.subjects?.map((s, i) => (
                  <span key={i} className="tag" style={{ marginRight: 4 }}>
                    {s.kind}:{s.name}
                  </span>
                )) || <span className="text-dim">—</span>}
              </td>
              <td><TimeAgo timestamp={rb.metadata.creationTimestamp} /></td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

// ── Cluster Roles ───────────────────────────────────────────────

function ClusterRolesTab() {
  const { data, loading } = useApi<K8sList<K8sClusterRole>>('/api/clusterroles');

  if (loading) return <div className="loading">Loading cluster roles…</div>;

  const items = (data?.items || []).filter(
    (cr) => !cr.metadata.name.startsWith('system:')
  );

  if (items.length === 0) {
    return <EmptyState icon="⊘" message="No custom cluster roles found." />;
  }

  return (
    <div className="table-wrap">
      <table className="data-table">
        <thead>
          <tr>
            <th>Name</th>
            <th>Rules</th>
            <th>Age</th>
          </tr>
        </thead>
        <tbody>
          {items.map((cr) => (
            <tr key={cr.metadata.name}>
              <td className="mono">{cr.metadata.name}</td>
              <td>{cr.rules?.length ?? 0}</td>
              <td><TimeAgo timestamp={cr.metadata.creationTimestamp} /></td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

// ── Cluster Role Bindings ───────────────────────────────────────

function ClusterRoleBindingsTab() {
  const { data, loading } = useApi<K8sList<K8sClusterRoleBinding>>('/api/clusterrolebindings');

  if (loading) return <div className="loading">Loading cluster role bindings…</div>;

  const items = (data?.items || []).filter(
    (crb) => !crb.metadata.name.startsWith('system:')
  );

  if (items.length === 0) {
    return <EmptyState icon="⊘" message="No custom cluster role bindings found." />;
  }

  return (
    <div className="table-wrap">
      <table className="data-table">
        <thead>
          <tr>
            <th>Name</th>
            <th>Role</th>
            <th>Subjects</th>
            <th>Age</th>
          </tr>
        </thead>
        <tbody>
          {items.map((crb) => (
            <tr key={crb.metadata.name}>
              <td className="mono">{crb.metadata.name}</td>
              <td className="mono">{crb.roleRef.kind}/{crb.roleRef.name}</td>
              <td>
                {crb.subjects?.map((s, i) => (
                  <span key={i} className="tag" style={{ marginRight: 4 }}>
                    {s.kind}:{s.name}
                  </span>
                )) || <span className="text-dim">—</span>}
              </td>
              <td><TimeAgo timestamp={crb.metadata.creationTimestamp} /></td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
