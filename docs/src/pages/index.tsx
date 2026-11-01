import type {ReactNode} from 'react';
import clsx from 'clsx';
import Link from '@docusaurus/Link';
import useDocusaurusContext from '@docusaurus/useDocusaurusContext';
import Layout from '@theme/Layout';
import Heading from '@theme/Heading';

import styles from './index.module.css';

function StudyDisclaimer() {
  return (
    <div
      style={{
        background: '#fef3c7',
        color: '#78350f',
        textAlign: 'center',
        padding: '0.6rem 1rem',
        fontSize: '0.9rem',
        borderBottom: '1px solid #f59e0b',
      }}>
      ⚠️ <strong>Study project</strong> — an educational DDD exercise. Not a
      production system.
    </div>
  );
}

function HomepageHeader() {
  const {siteConfig} = useDocusaurusContext();
  return (
    <header className={clsx('hero', styles.heroBanner)}>
      <StudyDisclaimer />
      <div className="container">
        <p className={styles.eyebrow}>warehouse-systems · Storefront · Generic subdomain</p>
        <Heading as="h1" className={styles.heroTitle}>
          {siteConfig.title}
        </Heading>
        <p className={styles.heroSubtitle}>{siteConfig.tagline}</p>
        <p className={styles.heroLead}>
          The only storefront edge: one stateless Go service that turns each
          screen and click of the shop into calls to six upstream REST services
          (product discovery, delivery promise, pricing, cart, checkout and
          customer ordering). It enforces a closed route allowlist, per-route
          time budgets, deterministic idempotency keys, a state cookie that is
          not authentication, and it refuses card data. It has no database, no
          events and logs no bodies.
        </p>
        <div className={styles.buttons}>
          <Link className="button button--primary button--lg" to="/docs/overview/introduction">
            Read the docs
          </Link>
          <Link
            className="button button--secondary button--lg"
            to="/docs/api-reference">
            API Reference
          </Link>
          <Link
            className="button button--secondary button--lg"
            to="/docs/adr">
            ADRs
          </Link>
        </div>
      </div>
    </header>
  );
}

export default function Home(): ReactNode {
  const {siteConfig} = useDocusaurusContext();
  return (
    <Layout
      title={siteConfig.title}
      description="Documentation for the Storefront BFF: the stateless storefront edge under /api/storefront that composes six upstream REST services, with its allowlist, time budgets, idempotency keys, state cookie and operations guides.">
      <HomepageHeader />
      <main>
        <section className={styles.invariant}>
          <div className="container">
            <blockquote className={styles.invariantQuote}>
              The BFF <strong>stores nothing and trusts nothing</strong>: it
              calls only <strong>25 allowlisted upstream routes</strong>, never
              retries a command, and no card number ever leaves the process.
            </blockquote>
            <p className={styles.invariantCaption}>
              <Link to="/docs/ecosystem/integration">The six upstreams →</Link>
            </p>
          </div>
        </section>
      </main>
    </Layout>
  );
}
