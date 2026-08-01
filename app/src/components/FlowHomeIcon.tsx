import { FLOW_HOME_COLORS } from '@/lib/colors';

/** Casa con tejado solar del diagrama de flujo. Inline para que currentColor siga al tema. */
export default function FlowHomeIcon({ className }: { className?: string }) {
  return (
    <svg viewBox="0 0 160 160" fill="none" xmlns="http://www.w3.org/2000/svg" role="img" className={className}>
      <defs>
        <linearGradient id="flow-home-roof-g" x1="30" y1="30" x2="130" y2="90" gradientUnits="userSpaceOnUse">
          <stop offset="0" stopColor={FLOW_HOME_COLORS.strokeFrom} />
          <stop offset="1" stopColor={FLOW_HOME_COLORS.strokeTo} />
        </linearGradient>
      </defs>
      {/* house body (slight isometric front) */}
      <path d="M34 84 L80 44 L126 84" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round" opacity="0.9" />
      {/* solar roof */}
      <path d="M46 76 L80 44 L114 76 Z" fill="url(#flow-home-roof-g)" />
      <g stroke={FLOW_HOME_COLORS.stroke} strokeWidth="1.4" opacity="0.85">
        <line x1="57.5" y1="65" x2="102.5" y2="65" />
        <line x1="69" y1="54.5" x2="91" y2="54.5" />
        <line x1="68.5" y1="49" x2="63" y2="76" />
        <line x1="91.5" y1="49" x2="97" y2="76" />
        <line x1="80" y1="44" x2="80" y2="76" />
      </g>
      {/* walls */}
      <path d="M40 84 V126 H120 V84" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round" fill="none" />
      {/* warm lit window */}
      <rect x="52" y="94" width="22" height="20" rx="3" fill={FLOW_HOME_COLORS.sun} />
      <g stroke={FLOW_HOME_COLORS.sunStroke} strokeWidth="1.4">
        <line x1="63" y1="94" x2="63" y2="114" />
        <line x1="52" y1="104" x2="74" y2="104" />
      </g>
      {/* door */}
      <path d="M92 126 V100 a8 8 0 0 1 16 0 V126" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" fill="none" opacity="0.85" />
      {/* ground line */}
      <line x1="28" y1="132" x2="132" y2="132" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" opacity="0.35" />
    </svg>
  );
}
