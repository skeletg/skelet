import { DetailPageSection, extendGroup, registerDetailSection, registerHomeWidget } from '@skelet/dashboard/sdk'

// Stands in for a real plugin's dashboard half in package/smoke.mjs, doing what docs/plugins.md tells one to do as
// it's imported: a page under one of core's menu groups, a home widget, a section on a record's page. The class names
// are ones the dashboard never uses, so finding them in the built CSS proves a plugin package's source got styled
extendGroup('catalog', [{ title: 'Smoke warranties', path: '/smoke-warranties', element: <p data-smoke="page">smoke page</p> }])

registerHomeWidget({
  id: 'smoke:widget',
  title: 'Smoke widget',
  Component: () => (
    <div data-smoke="widget" className="m-[13px] bg-[#ff00aa] p-[17px]">
      smoke widget
    </div>
  ),
})

registerDetailSection({
  id: 'smoke:customer',
  target: 'customer',
  placement: 'side',
  render: () => (
    <DetailPageSection title="Smoke section">
      <p data-smoke="side">smoke section</p>
    </DetailPageSection>
  ),
})
