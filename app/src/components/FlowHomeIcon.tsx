export default function FlowHomeIcon({ className }: { className?: string }) {
  return (
    <svg viewBox="0 0 160 160" fill="none" xmlns="http://www.w3.org/2000/svg" role="img" className={className}>
      <path d="M34 84 L80 44 L126 84" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round" />
      <path d="M46 76 L80 44 L114 76 Z" stroke="currentColor" strokeWidth="2" strokeLinejoin="round" fill="none" />
      <g stroke="currentColor" strokeWidth="1.4" opacity="0.7">
        <line x1="57.5" y1="65" x2="102.5" y2="65" />
        <line x1="69" y1="54.5" x2="91" y2="54.5" />
        <line x1="68.5" y1="49" x2="63" y2="76" />
        <line x1="91.5" y1="49" x2="97" y2="76" />
        <line x1="80" y1="44" x2="80" y2="76" />
      </g>
      <path d="M40 84 V126 H120 V84" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round" fill="none" />
      <rect x="52" y="94" width="22" height="20" rx="3" stroke="currentColor" strokeWidth="2" fill="none" />
      <g stroke="currentColor" strokeWidth="1.4">
        <line x1="63" y1="94" x2="63" y2="114" />
        <line x1="52" y1="104" x2="74" y2="104" />
      </g>
      <path d="M92 126 V100 a8 8 0 0 1 16 0 V126" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" fill="none" />
      <line x1="28" y1="132" x2="132" y2="132" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" opacity="0.35" />
    </svg>
  );
}
