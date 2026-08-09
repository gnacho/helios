import BrandLogoIcon from './BrandLogoIcon';

/** Logo de marca vectorizado (sigue el tema vía currentColor, patrón FlowHomeIcon). */
export default function BrandLogo({ className }: { className?: string }) {
  return <BrandLogoIcon className={className} />;
}
