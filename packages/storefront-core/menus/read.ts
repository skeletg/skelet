
import { perRequest } from "../request";
import { gqlFetch } from "../graphql";
import { surfaceCacheOpts, NAV_SURFACE, NAV_TAG } from "../config/read";
import { MenusDocument } from "./operations.gql";

export type MenuItem = {
  id: string;
  label: string;
  url: string | null;
  children: MenuItem[];
};

export type Menu = {
  id: string;
  slug: string;
  name: string;
  items: MenuItem[];
};

// Each component asks for the one menu it renders; a per-request DataLoader coalesces every call in a render pass into one menus(slugs)
// trip, so N menus cost one request Null when the menu is missing or unassigned to this store - consumers render nothing
export async function loadMenu(slug: string): Promise<Menu | null> {
  return getMenuLoader()(slug);
}

// One batcher per request
const getMenuLoader = perRequest(createMenuLoader);

type Waiter = {
  resolve: (menu: Menu | null) => void;
  reject: (err: unknown) => void;
};

function createMenuLoader(): (slug: string) => Promise<Menu | null> {
  const waiting = new Map<string, Waiter[]>();
  let scheduled = false;

  async function flush() {
    scheduled = false;
    const batch = new Map(waiting);
    waiting.clear();
    const slugs = [...batch.keys()];
    try {
      // Menus are store-scoped nav data → cached under the shared "nav" surface. The requested slug set rides in the
      // fetch body, so each distinct set is its own cache entry. Dev = always fresh
      const cacheOpts = await surfaceCacheOpts(NAV_SURFACE, { tags: [NAV_TAG] });
      const { data } = await gqlFetch(MenusDocument, { slugs }, cacheOpts);
      const bySlug = new Map<string, Menu>();
      for (const m of data.menus) {
        bySlug.set(m.slug, {
          id: m.id,
          slug: m.slug,
          name: m.name,
          items: m.items.map(hydrateItem),
        });
      }
      for (const [slug, waiters] of batch) {
        const menu = bySlug.get(slug) ?? null;
        for (const w of waiters) w.resolve(menu);
      }
    } catch (err) {
      // Preserve the previous throw-on-fetch-failure behaviour: a failed batch rejects
      // every caller rather than silently rendering empty
      for (const waiters of batch.values()) {
        for (const w of waiters) w.reject(err);
      }
    }
  }

  return function load(slug: string): Promise<Menu | null> {
    return new Promise<Menu | null>((resolve, reject) => {
      const waiters = waiting.get(slug);
      if (waiters) {
        waiters.push({ resolve, reject });
      } else {
        waiting.set(slug, [{ resolve, reject }]);
      }
      if (!scheduled) {
        scheduled = true;
        queueMicrotask(flush);
      }
    });
  };
}

// hydrateItem normalises each level of the GraphQL response (the deepest fetched level lacks a `children` field) into
// the uniform recursive MenuItem shape so renderers can descend without per-node guards
function hydrateItem(it: {
  id: string;
  label: string;
  url: string | null;
  children?: Array<{
    id: string;
    label: string;
    url: string | null;
    children?: Array<{
      id: string;
      label: string;
      url: string | null;
    }>;
  }>;
}): MenuItem {
  const kids = it.children ?? [];
  return {
    id: it.id,
    label: it.label,
    url: it.url ?? null,
    children: kids.map((k) => ({
      id: k.id,
      label: k.label,
      url: k.url ?? null,
      children: (k.children ?? []).map((kk) => ({
        id: kk.id,
        label: kk.label,
        url: kk.url ?? null,
        children: [],
      })),
    })),
  };
}
