import { useSearchParams } from "react-router-dom";
import { Tabs, TabsList, TabsTrigger } from "../../components/ui/tabs";
import { AboutYouPage } from "./about-you";
import { ProfileDetails } from "./profile-details";

/// Profile is the candidate's facts, under Details, and what they say about
/// themselves, under About you (docs/02_features/16_about_you.md). The two
/// save separately: one is the profile a form reads, the other the
/// material a letter is written from.
export function Profile() {
  const [params, setParams] = useSearchParams();
  const tab = params.get("tab") === "about" ? "about" : "details";
  const tabs = (
    <Tabs
      className="mb-6"
      value={tab}
      onValueChange={(value) =>
        setParams(
          (current) => {
            const next = new URLSearchParams(current);
            if (value === "about") next.set("tab", "about");
            else next.delete("tab");
            return next;
          },
          { replace: true },
        )
      }
    >
      <TabsList>
        <TabsTrigger value="details">Details</TabsTrigger>
        <TabsTrigger value="about">About you</TabsTrigger>
      </TabsList>
    </Tabs>
  );
  return tab === "about" ? <AboutYouPage tabs={tabs} /> : <ProfileDetails tabs={tabs} />;
}
