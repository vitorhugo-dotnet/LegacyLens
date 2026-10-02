import { useEffect, useState } from 'react';
import { createRoot } from 'react-dom/client';
import type { IndexResult, Project, ProjectListResult } from '@legacylens/contracts/src/protocol.ts';
import { NativeClient } from '../../src/native/client.ts';
import { InvestigationPage } from '../../src/investigation/InvestigationPage.tsx';
import { SearchPage } from '../../src/investigation/SearchPage.tsx';
import { ImpactPage } from '../../src/investigation/ImpactPage.tsx';
import '../../src/investigation/styles.css';

const client = new NativeClient();
const query = new URLSearchParams(location.search);
const traceId = query.get('traceId') ?? '';
const initialProjectId = query.get('projectId') ?? '';
const initialRevisionId = query.get('revisionId') ?? '';

function App() {
  const [projects, setProjects] = useState<Project[]>([]);
  const [projectId, setProjectId] = useState(initialProjectId);
  const [revisionId, setRevisionId] = useState(initialRevisionId);
  const [view, setView] = useState<'investigation' | 'search' | 'impact'>(traceId ? 'investigation' : 'search');
  const [root, setRoot] = useState('');
  const [name, setName] = useState('');
  const [status, setStatus] = useState('');
  const [error, setError] = useState('');
  const refresh = async () => {
    const all = new Map<string, Project>();
    let offset = 0;
    for (;;) {
      const page = await client.request<ProjectListResult>('project.list', { offset, limit: 200 });
      for (const project of page.items) all.set(project.id, project);
      if (!page.hasMore) break;
      offset += page.limit;
      if (offset > 1_000_000_000) throw new Error('Limite da lista de projetos atingido.');
    }
    setProjects([...all.values()]);
  };
  useEffect(() => { void refresh().catch((cause: unknown) => setError(cause instanceof Error ? cause.message : 'Host local indisponível.')); }, []);
  const register = async () => {
    setError('');
    try {
      const project = await client.request<Project>('project.register', { root, name });
      await refresh(); setProjectId(project.id); setStatus(`Projeto ${project.name} registrado.`);
    } catch (cause) { setError(cause instanceof Error ? cause.message : 'Falha ao registrar projeto.'); }
  };
  const index = async () => {
    if (!projectId) return;
    setError('');
    try {
      const result = await client.request<IndexResult>('project.index', { projectId, offset: 0, limit: 200 });
      setRevisionId(result.revisionId);
      setStatus(`Índice ${result.revisionId}: ${result.artifacts.total} artefatos, ${result.symbols.total} símbolos, ${result.relations.total} relações. ${result.artifacts.hasMore || result.symbols.hasMore || result.relations.hasMore ? 'Há mais resultados no índice.' : ''}`);
    } catch (cause) { setError(cause instanceof Error ? cause.message : 'Falha ao indexar projeto.'); }
  };
  return <>
    <nav className="project-tools"><h1>LegacyLens</h1>
      <label>Projeto <select value={projectId} onChange={(event) => { setProjectId(event.target.value); setRevisionId(''); }}><option value="">Selecione um projeto</option>{projects.map((project) => <option key={project.id} value={project.id}>{project.name}</option>)}</select></label>
      <button disabled={!projectId} onClick={() => void index()}>Indexar projeto</button>
      {projectId && <div role="group" aria-label="Ferramentas"><button aria-pressed={view === 'search'} onClick={() => setView('search')}>Buscar código</button><button aria-pressed={view === 'impact'} onClick={() => setView('impact')}>Impacto</button>{traceId && <button aria-pressed={view === 'investigation'} onClick={() => setView('investigation')}>Investigações</button>}</div>}
      <details><summary>Registrar projeto local</summary><label>Pasta do projeto <input value={root} onChange={(event) => setRoot(event.target.value)} /></label><label>Nome <input value={name} onChange={(event) => setName(event.target.value)} /></label><button disabled={!root.trim()} onClick={() => void register()}>Registrar</button></details>
      {status && <p role="status">{status}</p>}{error && <p role="alert">{error}</p>}
    </nav>
    {projectId && view === 'search' && <SearchPage client={client} projectId={projectId} revisionId={revisionId} />}
    {projectId && view === 'impact' && <ImpactPage client={client} projectId={projectId} revisionId={revisionId} />}
    {projectId && view === 'investigation' && traceId && <InvestigationPage client={client} projectId={projectId} traceId={traceId} />}
    {projectId && view === 'investigation' && !traceId && <p>Inicie uma captura na página da aplicação. Ao parar, a investigação abrirá aqui.</p>}
    {!projectId && <p>Selecione ou registre um projeto local.</p>}
  </>;
}

createRoot(document.getElementById('root')!).render(<App />);
