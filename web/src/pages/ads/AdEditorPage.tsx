import { ArrowLeft } from "lucide-react";
import { useEffect, useRef } from "react";
import { useForm, useWatch } from "react-hook-form";
import { Link, useNavigate, useParams } from "react-router";
import { useErrorText, useToast } from "../../components/feedback";
import { Button, Card, ErrorState, Field, LinkButton, PageHeader, Spinner, Textarea } from "../../components/ui";
import { useI18n } from "../../i18n";
import { useAd, useSaveAd } from "../../lib/queries";

/** The server's limit, which also sits well under Telegram's 4096 characters. */
const MAX_TEXT = 4000;

/**
 * The advert is its text. The groups receive it exactly as it is typed here;
 * the server only reads it to choose the groups, so there is nothing else to
 * fill in.
 */
export function AdEditorPage() {
  const { id } = useParams();
  const editing = Boolean(id);
  const { t } = useI18n();
  const navigate = useNavigate();
  const toast = useToast();
  const errorText = useErrorText();

  const existing = useAd(id);
  const save = useSaveAd(id);
  const loadedId = useRef<string | null>(null);

  const form = useForm<{ raw_text: string }>({ defaultValues: { raw_text: "" } });
  const invalid = Boolean(form.formState.errors.raw_text);
  const length = useWatch({ control: form.control, name: "raw_text" }).length;

  // Fill the form once per advert; a background refetch must not wipe edits.
  useEffect(() => {
    if (existing.data && loadedId.current !== existing.data.id) {
      loadedId.current = existing.data.id;
      form.reset({ raw_text: existing.data.raw_text ?? "" });
    }
  }, [existing.data, form]);

  // The text is sent as typed: trimming or reflowing it here would be changing
  // the advert behind the person's back.
  const submit = form.handleSubmit(({ raw_text }) => {
    save.mutate(
      { raw_text },
      {
        onSuccess: (ad) => {
          toast.success(t("ed.saved"));
          navigate(`/ads/${ad.id}`);
        },
        onError: (err) => toast.error(errorText(err)),
      },
    );
  });

  if (editing && existing.isLoading) return <Spinner />;
  if (editing && existing.error) return <ErrorState message={errorText(existing.error)} onRetry={() => void existing.refetch()} />;

  const back = editing ? `/ads/${id}` : "/ads";

  return (
    <>
      <PageHeader
        title={t(editing ? "ed.editTitle" : "ed.newTitle")}
        back={
          <Link to={back} className="mb-2 inline-flex items-center gap-1 text-sm text-muted hover:text-ink">
            <ArrowLeft className="size-4" aria-hidden />
            {t("common.back")}
          </Link>
        }
      />

      <form onSubmit={submit} noValidate>
        <Card>
          <Field label={t("ed.textLabel")} hint={t("ed.textHint")} error={invalid ? t("ed.textRequired") : undefined}>
            {(fieldId) => (
              <Textarea
                id={fieldId}
                rows={12}
                maxLength={MAX_TEXT}
                autoFocus={!editing}
                placeholder={t("ed.textPlaceholder")}
                invalid={invalid}
                {...form.register("raw_text", { validate: (v) => v.trim() !== "" })}
              />
            )}
          </Field>
          <div className="mt-4 flex flex-wrap items-center justify-between gap-3">
            <span className="text-xs text-muted tabular">
              {length} / {MAX_TEXT}
            </span>
            <div className="flex gap-2">
              <LinkButton to={back} variant="secondary">
                {t("common.cancel")}
              </LinkButton>
              <Button type="submit" loading={save.isPending}>
                {save.isPending ? t("common.saving") : t("common.save")}
              </Button>
            </div>
          </div>
        </Card>
      </form>
    </>
  );
}
