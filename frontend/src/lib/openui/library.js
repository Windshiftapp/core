// The OpenUI component library for custom dashboard widgets.
//
// This file is the single source of truth for what a generated program may
// use: the runtime library, the prompt, and the Go-side allow-list in
// internal/customwidgets all have to agree. frontend/scripts/check-openui-prompt.mjs
// fails the build when the generated prompt drifts from this spec.
//
// Components are display-only. There is deliberately no HTML, image, iframe,
// or arbitrary-URL component, and no component performs a mutation. Data-bound
// components declare a CQL query and read their rows from the widget data
// context; the program never carries workspace, user, or asset-set IDs.

import { createLibrary, defineComponent } from '@openuidev/svelte-lang';
import { z } from 'zod/v4';

import ActivityFeed from './components/ActivityFeed.svelte';
import Badge from './components/Badge.svelte';
import BarChart from './components/BarChart.svelte';
import BipartiteList from './components/BipartiteList.svelte';
import Card from './components/Card.svelte';
import Divider from './components/Divider.svelte';
import EmptyState from './components/EmptyState.svelte';
import ItemList from './components/ItemList.svelte';
import ItemsTable from './components/ItemsTable.svelte';
import LineChart from './components/LineChart.svelte';
import LinkButton from './components/LinkButton.svelte';
import Matrix from './components/Matrix.svelte';
import Metric from './components/Metric.svelte';
import MetricRow from './components/MetricRow.svelte';
import NetworkGraph from './components/NetworkGraph.svelte';
import PieChart from './components/PieChart.svelte';
import ProgressList from './components/ProgressList.svelte';
import Section from './components/Section.svelte';
import Stack from './components/Stack.svelte';
import Text from './components/Text.svelte';
import Title from './components/Title.svelte';

const entity = z
  .enum(['item', 'asset'])
  .optional()
  .describe('Which entity to query. Assets require the widget to have an asset set selected.');

const query = z
  .string()
  .optional()
  .describe(
    'CQL filter expression, for example "status_completed = false AND assignee = currentUser()".'
  );

const sort = z
  .string()
  .optional()
  .describe('Sort field, optionally prefixed with "-" for descending, for example "-updated_at".');

const limit = z
  .number()
  .int()
  .min(1)
  .max(1000)
  .optional()
  .describe('Maximum rows to fetch, 1-1000. Use 1000 only for charts and aggregations.');

const TitleDef = defineComponent({
  name: 'Title',
  description: 'A short heading inside a card or section.',
  props: z.object({
    text: z.string().describe('Heading text.'),
    level: z.enum(['1', '2', '3']).optional().describe('Visual size; 1 is largest. Defaults to 2.'),
  }),
  component: Title,
});

const TextDef = defineComponent({
  name: 'Text',
  description: 'A paragraph of plain text. Use for context or explanation, not for data.',
  props: z.object({
    text: z.string().describe('The text to display.'),
    tone: z.enum(['default', 'muted']).optional().describe('Muted renders secondary text.'),
  }),
  component: Text,
});

const DividerDef = defineComponent({
  name: 'Divider',
  description: 'A horizontal separator between blocks.',
  props: z.object({}),
  component: Divider,
});

const BadgeDef = defineComponent({
  name: 'Badge',
  description: 'A small status pill.',
  props: z.object({
    text: z.string().describe('Badge label.'),
    tone: z.enum(['neutral', 'info', 'success', 'warning', 'danger']).optional(),
  }),
  component: Badge,
});

const EmptyStateDef = defineComponent({
  name: 'EmptyState',
  description: 'Shown when a query returns nothing.',
  props: z.object({
    message: z.string().describe('Message explaining that there is no data.'),
  }),
  component: EmptyState,
});

const LinkButtonDef = defineComponent({
  name: 'LinkButton',
  description:
    'A button that opens an item or navigates to an in-app route. href must be a relative application path.',
  props: z.object({
    label: z.string().describe('Button label.'),
    href: z
      .string()
      .optional()
      .describe('Relative in-app path such as "/items". Off-site URLs are rejected.'),
    item_id: z.number().int().optional().describe('Work item ID to open in the item dialog.'),
  }),
  component: LinkButton,
});

const MetricDef = defineComponent({
  name: 'Metric',
  description:
    'A single number from a query. aggregate="count" uses the exact server total; "sum" adds a numeric field over the fetched rows.',
  props: z.object({
    label: z.string().describe('Metric label.'),
    query,
    aggregate: z.enum(['count', 'sum']).optional().describe('Defaults to count.'),
    field: z.string().optional().describe('Numeric field to sum when aggregate is "sum".'),
    entity,
    sort,
    limit,
  }),
  component: Metric,
});

const MetricRowDef = defineComponent({
  name: 'MetricRow',
  description: 'A row of metrics.',
  props: z.object({
    children: z.array(MetricDef.ref).describe('The metrics to show side by side.'),
  }),
  component: MetricRow,
});

const ItemsTableDef = defineComponent({
  name: 'ItemsTable',
  description:
    'A table of items or assets matching a query. The key and title columns link to the item.',
  props: z.object({
    query,
    columns: z
      .string()
      .optional()
      .describe('Comma-separated field names, for example "key,title,status,assignee,due_date".'),
    sort,
    limit,
    entity,
  }),
  component: ItemsTable,
});

const ItemListDef = defineComponent({
  name: 'ItemList',
  description: 'A compact list of items or assets with a status column.',
  props: z.object({ query, sort, limit, entity }),
  component: ItemList,
});

const BarChartDef = defineComponent({
  name: 'BarChart',
  description:
    'Counts or sums grouped by a field. Aggregation runs in the browser over up to the row limit; set sort only when the request asks for an order.',
  props: z.object({
    query,
    group_by: z.string().describe('Field to group by, for example "status" or "assignee".'),
    sort,
    limit,
    metric: z.enum(['count', 'sum']).optional().describe('Defaults to count.'),
    field: z.string().optional().describe('Numeric field to sum when metric is "sum".'),
    group_limit: z
      .number()
      .int()
      .min(1)
      .max(50)
      .optional()
      .describe('Maximum bars. Defaults to 10.'),
    entity,
    link_mode: z
      .enum(['fanout', 'primary'])
      .optional()
      .describe(
        'Only for group_by fields on linked_items: count a row once per link (fanout) or once in total (primary). Defaults to fanout.'
      ),
  }),
  component: BarChart,
});

const LineChartDef = defineComponent({
  name: 'LineChart',
  description:
    'A trend over time. Buckets rows by a date field in the browser; set sort only when the request asks for an order, and give it a generous limit.',
  props: z.object({
    query,
    field: z.string().optional().describe('Date field to bucket on. Defaults to "created_at".'),
    bucket: z.enum(['day', 'week', 'month']).optional().describe('Bucket size. Defaults to week.'),
    sort,
    limit,
    metric: z.enum(['count', 'sum']).optional().describe('Defaults to count.'),
    entity,
  }),
  component: LineChart,
});

const PieChartDef = defineComponent({
  name: 'PieChart',
  description:
    'A share-of-total pie with one slice per value of group_by. Only use it when the request explicitly asks for a pie or donut; BarChart is easier to compare.',
  props: z.object({
    query,
    group_by: z.string().describe('Field to group by, for example "status" or "priority".'),
    sort,
    limit,
    metric: z.enum(['count', 'sum']).optional().describe('Defaults to count.'),
    field: z.string().optional().describe('Numeric field to sum when metric is "sum".'),
    group_limit: z
      .number()
      .int()
      .min(1)
      .max(20)
      .optional()
      .describe('Maximum slices. Defaults to 8.'),
    donut: z.boolean().optional().describe('Draw a donut with the total in the centre.'),
    entity,
    link_mode: z
      .enum(['fanout', 'primary'])
      .optional()
      .describe('Only for group_by fields on linked_items. Defaults to fanout.'),
  }),
  component: PieChart,
});

const ProgressListDef = defineComponent({
  name: 'ProgressList',
  description: 'Share-of-total bars, one row per distinct value of group_by.',
  props: z.object({
    query,
    group_by: z.string().describe('Field to group by.'),
    sort,
    limit,
    metric: z.enum(['count', 'sum']).optional(),
    field: z.string().optional(),
    group_limit: z.number().int().min(1).max(50).optional(),
    entity,
    link_mode: z
      .enum(['fanout', 'primary'])
      .optional()
      .describe('Only for group_by fields on linked_items. Defaults to fanout.'),
  }),
  component: ProgressList,
});

const ActivityFeedDef = defineComponent({
  name: 'ActivityFeed',
  description: 'A reverse-chronological list of recently updated items or assets.',
  props: z.object({ query, sort, limit, entity }),
  component: ActivityFeed,
});

// The graph components all read the same bipartite edge slot: an item seed
// set, an asset seed set, and the links between them. The edge cap and the
// render cap are separate, because the data can be complete while the drawing
// is sampled.
const graphSeedProps = {
  item_query: z.string().optional().describe('CQL filter for the item side of the graph.'),
  asset_query: z.string().optional().describe('CQL filter for the asset side of the graph.'),
  item_sort: z
    .string()
    .optional()
    .describe('Sort field for the item side, "-" prefixed for descending.'),
  item_limit: z
    .number()
    .int()
    .min(1)
    .max(1000)
    .optional()
    .describe('Maximum items on the item side. Defaults to 1000.'),
  asset_sort: z
    .string()
    .optional()
    .describe('Sort field for the asset side, "-" prefixed for descending.'),
  asset_limit: z
    .number()
    .int()
    .min(1)
    .max(1000)
    .optional()
    .describe('Maximum assets on the asset side. Defaults to 1000.'),
  max_edges: z
    .number()
    .int()
    .min(1)
    .max(10000)
    .optional()
    .describe('Maximum edges to resolve. Defaults to 10000.'),
};

const renderedNodesProp = {
  rendered_nodes: z
    .number()
    .int()
    .min(10)
    .max(1000)
    .optional()
    .describe(
      'Maximum nodes drawn per side. Defaults to 200; the data can be complete while the drawing is sampled.'
    ),
};

const BipartiteListDef = defineComponent({
  name: 'BipartiteList',
  description:
    'A readable list of item-to-asset links. Use this unless the user explicitly asks for a chart; it stays usable at any size.',
  props: z.object(graphSeedProps),
  component: BipartiteList,
});

const NetworkGraphDef = defineComponent({
  name: 'NetworkGraph',
  description:
    'Items in one column, assets in another, links drawn between them. Draws at most rendered_nodes per side.',
  props: z.object({ ...graphSeedProps, ...renderedNodesProp }),
  component: NetworkGraph,
});

const MatrixDef = defineComponent({
  name: 'Matrix',
  description:
    'An item-by-asset matrix where a filled cell is a link. Best when both sides are large; draws at most rendered_nodes per side.',
  props: z.object({ ...graphSeedProps, ...renderedNodesProp }),
  component: Matrix,
});

// Containers are declared in nesting order: Stack holds content, Card holds
// content plus Stack, Section holds all of them. Ordering avoids recursive
// schemas, which the prompt generator does not support.
const contentDefs = [
  TitleDef,
  TextDef,
  DividerDef,
  BadgeDef,
  EmptyStateDef,
  LinkButtonDef,
  MetricDef,
  MetricRowDef,
  ItemsTableDef,
  ItemListDef,
  BarChartDef,
  LineChartDef,
  PieChartDef,
  ProgressListDef,
  ActivityFeedDef,
  BipartiteListDef,
  NetworkGraphDef,
  MatrixDef,
];
const contentChildren = contentDefs.map((definition) => definition.ref);

const StackDef = defineComponent({
  name: 'Stack',
  description: 'Lays its children out in a vertical or horizontal stack.',
  props: z.object({
    children: z.array(z.union(contentChildren)).describe('The stacked blocks.'),
    direction: z.enum(['vertical', 'horizontal']).optional(),
    gap: z
      .enum(['none', 'sm', 'md', 'lg'])
      .optional()
      .describe('Spacing between children. Defaults to md.'),
  }),
  component: Stack,
});

const CardDef = defineComponent({
  name: 'Card',
  description: 'A bordered surface that groups related blocks. Usually the root of a widget.',
  props: z.object({
    children: z.array(z.union([...contentChildren, StackDef.ref])).describe('The card contents.'),
    padding: z.enum(['none', 'compact', 'default', 'spacious']).optional(),
  }),
  component: Card,
});

const SectionDef = defineComponent({
  name: 'Section',
  description: 'A titled group of blocks, optionally nested inside a Card.',
  props: z.object({
    heading: z.string().optional().describe('Section heading.'),
    children: z
      .array(z.union([...contentChildren, StackDef.ref, CardDef.ref]))
      .describe('The section contents.'),
  }),
  component: Section,
});

export const widgetLibrary = createLibrary({
  components: [...contentDefs, StackDef, CardDef, SectionDef],
  componentGroups: [
    {
      name: 'Layout',
      components: ['Stack', 'Card', 'Section', 'Divider'],
      notes: ['Every widget starts from a root statement, usually root = Card(...).'],
    },
    { name: 'Content', components: ['Title', 'Text', 'Badge', 'EmptyState', 'LinkButton'] },
    {
      name: 'Graph',
      components: ['BipartiteList', 'NetworkGraph', 'Matrix'],
      notes: [
        'Graph components take item_query and asset_query and return the links between those two sets. Prefer BipartiteList; a graph at more than a few hundred nodes is not readable.',
      ],
    },
    {
      name: 'Data',
      components: [
        'Metric',
        'MetricRow',
        'ItemsTable',
        'ItemList',
        'BarChart',
        'LineChart',
        'PieChart',
        'ProgressList',
        'ActivityFeed',
      ],
      notes: [
        'Data components take a CQL query. Never put workspace, user, or asset-set IDs in the query; scope is applied by the viewer session.',
      ],
    },
  ],
  root: 'Card',
});

/** Component names allowed in a program. Mirrors internal/customwidgets. */
export const componentNames = [
  'ActivityFeed',
  'Badge',
  'BarChart',
  'BipartiteList',
  'Card',
  'Divider',
  'EmptyState',
  'ItemList',
  'ItemsTable',
  'LineChart',
  'LinkButton',
  'Matrix',
  'Metric',
  'MetricRow',
  'NetworkGraph',
  'PieChart',
  'ProgressList',
  'Section',
  'Stack',
  'Text',
  'Title',
];
