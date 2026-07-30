export default function Footer() {
  return (
    <footer className="mx-auto w-full max-w-[1440px] px-4 pb-6 pt-2 lg:px-6">
      <div className="flex flex-col items-center justify-between gap-2 border-t border-app pt-4 text-xs text-faint sm:flex-row">
        <p>Helios · Monitor Solar — datos locales vía Home Assistant</p>
        <p>Solis 4,4 kWp + Fox 2,7 kWp · Soluna 5 kWh</p>
      </div>
    </footer>
  );
}
