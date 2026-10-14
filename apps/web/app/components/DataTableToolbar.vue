<template>
  <form
    novalidate
    data-slot="data-table-toolbar"
    class="flex w-full flex-wrap items-start gap-2 [&_input]:h-8 [&_[data-slot=select-trigger]]:h-8 [&_[data-slot=button]]:h-8"
  >
    <slot />
  </form>
</template>
