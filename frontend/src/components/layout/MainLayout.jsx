import {Outlet} from "react-router-dom";
import {Sidebar} from "./Sidebar";

export function MainLayout({health}) {
  return (
    <div className="min-h-screen bg-background text-foreground">
      <Sidebar health={health} />
      <main className="ml-72 min-h-screen">
        <Outlet />
      </main>
    </div>
  );
}
