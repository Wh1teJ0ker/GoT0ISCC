import {useEffect, useState} from "react";
import {NavLink, useLocation} from "react-router-dom";
import {ChevronDown, Github, RadioTower} from "lucide-react";
import logo from "../../assets/images/logo-universal.png";
import {navItems} from "../../lib/iscc";
import {cn} from "../../lib/utils";
import {Button} from "../ui/Button";

export function Sidebar({health}) {
  const location = useLocation();
  const theoryActive = location.pathname.startsWith("/theory");
  const [openGroups, setOpenGroups] = useState(() => ({theory: theoryActive}));

  useEffect(() => {
    if (theoryActive) {
      setOpenGroups((current) => ({...current, theory: true}));
    }
  }, [theoryActive]);

  function toggleGroup(key) {
    setOpenGroups((current) => ({...current, [key]: !current[key]}));
  }

  return (
    <aside className="fixed left-0 top-0 z-20 flex h-screen w-72 flex-col border-r border-border bg-card text-card-foreground">
      <div className="border-b border-border p-5">
        <div className="flex items-center gap-3">
          <img className="h-10 w-10 rounded-md border border-border bg-background object-cover" src={logo} alt="GoT0ISCC" />
          <div className="min-w-0">
            <h1 className="truncate text-lg font-bold tracking-normal">GoT0ISCC</h1>
            <p className="truncate text-xs text-muted-foreground">ISCC Desktop Console</p>
          </div>
        </div>
      </div>

      <nav className="flex-1 space-y-1 overflow-y-auto p-3">
        {navItems.map((item) => {
          if (!item.children?.length) {
            return (
              <NavLink
                key={item.key}
                to={item.path}
                end={item.path === "/"}
                className={({isActive}) =>
                  cn(
                    "group flex items-center gap-3 rounded-md px-3 py-2.5 text-sm font-medium text-muted-foreground transition-colors",
                    "hover:bg-accent hover:text-accent-foreground",
                    isActive && "bg-secondary text-secondary-foreground",
                  )
                }
              >
                <item.icon className="h-4 w-4 shrink-0" />
                <span className="truncate">{item.label}</span>
              </NavLink>
            );
          }

          const groupOpen = Boolean(openGroups[item.key]);
          const groupActive = location.pathname.startsWith(item.path);

          return (
            <div key={item.key} className="space-y-1">
              <button
                type="button"
                onClick={() => toggleGroup(item.key)}
                className={cn(
                  "group flex w-full items-center gap-3 rounded-md px-3 py-2.5 text-left text-sm font-medium transition-colors",
                  groupActive
                    ? "bg-secondary text-secondary-foreground"
                    : "text-muted-foreground hover:bg-accent hover:text-accent-foreground",
                )}
              >
                <item.icon className="h-4 w-4 shrink-0" />
                <span className="min-w-0 flex-1 truncate">{item.label}</span>
                <ChevronDown className={cn("h-4 w-4 shrink-0 transition-transform", groupOpen && "rotate-180")} />
              </button>

              {groupOpen ? (
                <div className="space-y-1 border-l border-border/70 pl-4 ml-5">
                  {item.children.map((child) => (
                    <NavLink
                      key={child.key}
                      to={child.path}
                      className={({isActive}) =>
                        cn(
                          "flex items-center rounded-md px-3 py-2 text-sm transition-colors",
                          isActive
                            ? "bg-accent text-accent-foreground"
                            : "text-muted-foreground hover:bg-accent/70 hover:text-accent-foreground",
                        )
                      }
                    >
                      <span className="truncate">{child.label}</span>
                    </NavLink>
                  ))}
                </div>
              ) : null}
            </div>
          );
        })}
      </nav>

      <div className="space-y-3 border-t border-border p-4">
        <div className="rounded-md bg-muted/60 p-3">
          <div className="mb-2 flex items-center gap-2 text-xs font-medium text-muted-foreground">
            <RadioTower className="h-3.5 w-3.5" />
            <span>运行状态</span>
          </div>
          <div className="text-sm font-semibold">{health?.status || "loading"}</div>
          <div className="mt-1 text-xs text-muted-foreground">Wails + Go + Python Sandbox</div>
        </div>
        <Button variant="outline" className="w-full justify-start" size="sm">
          <Github className="h-4 w-4" />
          Workspace
        </Button>
      </div>
    </aside>
  );
}
