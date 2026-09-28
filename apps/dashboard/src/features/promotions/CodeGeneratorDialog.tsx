import { useMutation } from '@apollo/client/react'
import { FormDialog } from '@/components/form/FormDialog'
import { FormDialogBody } from '@/components/form/FormDialogBody'
import { useDialogForm } from '@/components/form/useDialogForm'
import { GeneratePromotionCodesDocument } from './Promotions.gql'
import { codeGeneratorSchema, defaultCodeGenerator, toCodeGeneratorVariables } from './schemas/codeGenerator'

// The backend's pattern-based bulk code generator - see schemas/codeGenerator for the grammar
export function CodeGeneratorDialog({
  ruleId,
  open,
  onOpenChange,
  onGenerated,
}: {
  ruleId: string
  open: boolean
  onOpenChange: (open: boolean) => void
  onGenerated?: () => void
}) {
  return (
    <FormDialog open={open} onOpenChange={onOpenChange}>
      {(close) => <CodeGeneratorForm ruleId={ruleId} onGenerated={onGenerated} onClose={close} />}
    </FormDialog>
  )
}

function CodeGeneratorForm({ ruleId, onGenerated, onClose }: { ruleId: string; onGenerated?: () => void; onClose: () => void }) {
  const [generate] = useMutation(GeneratePromotionCodesDocument)
  const dialog = useDialogForm({
    baseline: defaultCodeGenerator,
    schema: codeGeneratorSchema,
    submit: async (v) => {
      await generate({ variables: toCodeGeneratorVariables(v, ruleId) })
    },
    onSaved: () => {
      onGenerated?.()
      onClose()
    },
  })
  const { form } = dialog

  return (
    <FormDialogBody
      dialog={dialog}
      title="Generate codes"
      description={
        <>
          Pattern grammar: <code>[A]</code> uppercase letter, <code>[N]</code> digit, <code>[*]</code> alphanumeric. Everything
          else is literal.
        </>
      }
      submitLabel="Generate"
      onCancel={onClose}
    >
      <form.AppField name="pattern">{(f) => <f.TextField id="pattern" label="Pattern" className="font-mono" />}</form.AppField>
      <form.AppField name="count">{(f) => <f.NumberField id="count" label="Count" min={1} max={10000} />}</form.AppField>
      <div className="grid grid-cols-2 items-start gap-3">
        <form.AppField name="prefix">{(f) => <f.TextField id="prefix" label="Prefix (optional)" />}</form.AppField>
        <form.AppField name="suffix">{(f) => <f.TextField id="suffix" label="Suffix (optional)" />}</form.AppField>
      </div>
    </FormDialogBody>
  )
}
