import { type ReactNode } from 'react'
import { NavLink } from 'react-router-dom'

const navItems = [
  { to: '/scenarios', label: 'Scenarios' },
  { to: '/live', label: 'Live' },
  { to: '/sql', label: 'SQL' },
  { to: '/history', label: 'History' },
  { to: '/compare', label: 'Compare' },
  { to: '/trends', label: 'Trends' },
  { to: '/monitoring', label: 'Monitoring' },
]

const styles = {
  container: {
    display: 'flex',
    height: '100vh',
    overflow: 'hidden',
  } as const,
  sidebar: {
    width: 200,
    flexShrink: 0,
    background: '#111',
    borderRight: '1px solid var(--border)',
    display: 'flex',
    flexDirection: 'column',
    padding: '20px 0',
  } as const,
  logo: {
    padding: '0 16px 24px',
    fontSize: 15,
    fontWeight: 600,
    color: 'var(--accent)',
    lineHeight: 1.3,
    letterSpacing: '-0.3px',
  } as const,
  nav: {
    display: 'flex',
    flexDirection: 'column',
    gap: 2,
  } as const,
  link: {
    display: 'block',
    padding: '8px 16px',
    color: 'var(--text-secondary)',
    textDecoration: 'none',
    fontSize: 13,
    fontWeight: 500,
    borderLeft: '3px solid transparent',
    transition: 'all 0.15s',
  } as const,
  activeLink: {
    color: 'var(--text-primary)',
    borderLeftColor: 'var(--accent)',
    background: 'var(--bg-hover)',
  } as const,
  main: {
    flex: 1,
    overflow: 'auto',
    padding: 24,
  } as const,
}

export function Layout({ children }: { children: ReactNode }) {
  return (
    <div style={styles.container}>
      <aside style={styles.sidebar}>
        <div style={styles.logo}>CockroachDB<br />Benchmarks</div>
        <nav style={styles.nav}>
          {navItems.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              style={({ isActive }) => ({
                ...styles.link,
                ...(isActive ? styles.activeLink : {}),
              })}
            >
              {item.label}
            </NavLink>
          ))}
        </nav>
      </aside>
      <main style={styles.main}>{children}</main>
    </div>
  )
}
