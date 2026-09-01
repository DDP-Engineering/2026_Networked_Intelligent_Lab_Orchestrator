import './App.css';

function App() {
  return (
    <div className="app-frame">
      <header className="topbar">
        <div className="brand">
          <span className="brand-mark" aria-hidden="true">
            N
          </span>
          <div>
            <h1>NILO</h1>
            <p>Networked Intelligent Lab Orchestrator</p>
          </div>
        </div>
        <span className="simulation-status" role="status" aria-label="Runtime profile">
          <span className="status-dot" aria-hidden="true" />
          SIMULATION
        </span>
      </header>

      <main className="workspace">
        <section className="hero" aria-labelledby="workspace-title">
          <p className="eyebrow">Development interface</p>
          <h2 id="workspace-title">Frontend foundation is ready.</h2>
          <p className="hero-copy">
            The native Windows toolchain can now develop, test, and build NILO&apos;s
            browser interface. Backend authentication and live bench data arrive in the
            next slice.
          </p>

          <dl className="foundation-grid">
            <div className="foundation-card foundation-card-ready">
              <dt>Frontend</dt>
              <dd>Ready</dd>
              <p>React and TypeScript build locally from the locked repository.</p>
            </div>
            <div className="foundation-card">
              <dt>Bench Core</dt>
              <dd>Not connected</dd>
              <p>This scaffold deliberately makes no backend request.</p>
            </div>
            <div className="foundation-card">
              <dt>Equipment</dt>
              <dd>Simulation only</dd>
              <p>No physical electronics or automation network is contacted.</p>
            </div>
          </dl>
        </section>

        <aside className="next-step" aria-labelledby="next-step-title">
          <span className="step-index" aria-hidden="true">
            04
          </span>
          <div>
            <p className="eyebrow">Next vertical slice</p>
            <h2 id="next-step-title">Login and bench status</h2>
            <p>
              The next increment connects this shell to the authenticated simulation API
              and renders integration and device state.
            </p>
          </div>
        </aside>
      </main>
    </div>
  );
}

export default App;
