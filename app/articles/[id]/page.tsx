import { client, type Blog } from "@/lib/microcms";     
import { Card, CardContent } from "@/components/ui/card";                                                                                                   
                                                                                                                                                             
const ArticlePage = async ({ params }: { params: Promise<{ id: string }> }) => {                                                                           
  const { id } = await params;
  const blog = await client.get<Blog>({                                                                                                                    
    endpoint: "blogs",
    contentId: id,                                                                                                                                         
  });                                                                                                                                                    

  return (
    <main className="w-full mx-auto max-w-3xl p-8">
    <Card>
      <CardContent className="space-y-4">
        <h1 className="text-3xl font-bold">{blog.title}</h1>
        <time className="text-sm text-muted-foreground">
          {new Date(blog.createdAt).toLocaleDateString("ja-JP")}
        </time>
        <div
          className="prose"
          dangerouslySetInnerHTML={{ __html: blog.content }}
        />
      </CardContent>
    </Card>
  </main>                                                                                                                                       
  );
};

export default ArticlePage;